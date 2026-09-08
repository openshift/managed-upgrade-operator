package metrics

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gomock "go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

func newMockMetricsClient(server *httptest.Server) *Counter {
	mockCtrl := gomock.NewController(GinkgoT())
	mockClient := mocks.NewMockClient(mockCtrl)

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	svcPort := int32(port) //nolint:gosec

	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})

	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, _ interface{}, obj interface{}, _ ...interface{}) error {
			switch o := obj.(type) {
			case *corev1.Service:
				*o = corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      promApp,
						Namespace: MonitoringNS,
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "web", Port: svcPort},
						},
					},
				}
			case *corev1.ConfigMap:
				*o = corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      MonitoringCAConfigMapName,
						Namespace: MonitoringNS,
					},
					Data: map[string]string{
						MonitoringConfigField: string(caCertPEM),
					},
				}
			}
			return nil
		},
	).AnyTimes()

	mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, obj interface{}, _ ...interface{}) error {
			if sl, ok := obj.(*corev1.SecretList); ok {
				sl.Items = []corev1.Secret{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "prometheus-k8s-token-test"},
						Data:       map[string][]byte{corev1.ServiceAccountTokenKey: []byte("test-token")},
					},
				}
			}
			return nil
		},
	).AnyTimes()

	mc, err := NewBuilder().NewClient(mockClient)
	Expect(err).NotTo(HaveOccurred())

	counter, ok := mc.(*Counter)
	Expect(ok).To(BeTrue())
	counter.promTarget = u.Host

	return counter
}

var _ = Describe("Counter", func() {
	var (
		server *httptest.Server
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("does not leak goroutines from Query", func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		}))
		counter := newMockMetricsClient(server)

		runtime.GC()
		before := runtime.NumGoroutine()

		for i := 0; i < 100; i++ {
			_, err := counter.Query("up")
			Expect(err).NotTo(HaveOccurred())
		}

		runtime.GC()
		after := runtime.NumGoroutine()
		leaked := after - before
		Expect(leaked).To(BeNumerically("<=", 10),
			"goroutine leak: %d before, %d after (%d leaked)", before, after, leaked)
	})

	It("returns parsed results from Query", func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/api/v1/query"))
			Expect(r.URL.Query().Get("query")).To(Equal("up"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"metric":{"alertname":"TestAlert"},"value":[1,"1"]}]}}`))
		}))
		counter := newMockMetricsClient(server)

		resp, err := counter.Query("up")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Data.Result).To(HaveLen(1))
		Expect(resp.Data.Result[0].Metric["alertname"]).To(Equal("TestAlert"))
	})

	It("sets the Authorization header on Query", func() {
		server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		}))
		counter := newMockMetricsClient(server)

		_, err := counter.Query("up")
		Expect(err).NotTo(HaveOccurred())
	})
})

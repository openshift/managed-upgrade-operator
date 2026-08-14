package dvo

import (
	"encoding/json"
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

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

func newMockDvoClient(server *httptest.Server) DvoClient {
	mockCtrl := gomock.NewController(GinkgoT())
	mockClient := mocks.NewMockClient(mockCtrl)

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	svcPort := int32(port) //nolint:gosec

	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, _ interface{}, obj interface{}, _ ...interface{}) error {
			switch o := obj.(type) {
			case *corev1.Service:
				*o = corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "deployment-validation-operator-metrics",
						Namespace: "openshift-deployment-validation-operator",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{Name: "http-metrics", Port: svcPort},
						},
					},
				}
			case *configv1.ClusterVersion:
				*o = configv1.ClusterVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "version"},
					Spec:       configv1.ClusterVersionSpec{ClusterID: "test-cluster"},
				}
			case *corev1.Secret:
				pullSecret, _ := json.Marshal(map[string]interface{}{
					"auths": map[string]interface{}{
						"cloud.openshift.com": map[string]interface{}{
							"auth": "test-pull-secret",
						},
					},
				})
				*o = corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "pull-secret", Namespace: "openshift-config"},
					Data:       map[string][]byte{".dockerconfigjson": pullSecret},
				}
			}
			return nil
		},
	).AnyTimes()

	dc, err := NewBuilder().New(mockClient)
	Expect(err).NotTo(HaveOccurred())

	realClient, ok := dc.(*dvoClient)
	Expect(ok).To(BeTrue())
	realClient.dvoBaseUrl = u.Host

	return dc
}

var _ = Describe("dvoClient", func() {
	var (
		server *httptest.Server
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("does not leak goroutines from GetMetrics", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("# HELP some_metric\nsome_metric 1\n"))
		}))
		dc := newMockDvoClient(server)

		runtime.GC()
		before := runtime.NumGoroutine()

		for i := 0; i < 100; i++ {
			_, err := dc.GetMetrics()
			Expect(err).NotTo(HaveOccurred())
		}

		runtime.GC()
		after := runtime.NumGoroutine()
		leaked := after - before
		Expect(leaked).To(BeNumerically("<=", 10),
			"goroutine leak: %d before, %d after (%d leaked)", before, after, leaked)
	})

	It("returns the response body from GetMetrics", func() {
		expected := "# HELP some_metric\nsome_metric 1\n"
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(r.URL.Path).To(Equal("/metrics"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(expected))
		}))
		dc := newMockDvoClient(server)

		body, err := dc.GetMetrics()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal(expected))
	})

	It("sets the Authorization header on GetMetrics", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			Expect(auth).NotTo(BeEmpty())
			Expect(auth).To(Equal("AccessToken test-cluster:test-pull-secret"))
			w.WriteHeader(http.StatusOK)
		}))
		dc := newMockDvoClient(server)

		_, err := dc.GetMetrics()
		Expect(err).NotTo(HaveOccurred())
	})
})

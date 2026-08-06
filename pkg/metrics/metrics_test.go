package metrics

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"testing"

	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

func newMockMetricsClient(t *testing.T, server *httptest.Server) *Counter {
	t.Helper()

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockClient := mocks.NewMockClient(mockCtrl)

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	svcPort := int32(port) //nolint:gosec // test port from httptest, no overflow risk

	// Extract the test server's CA cert as PEM for MonitoringTLSConfig
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
	if err != nil {
		t.Fatalf("failed to build metrics client: %v", err)
	}

	counter, ok := mc.(*Counter)
	if !ok {
		t.Fatal("unexpected Metrics implementation type")
	}
	counter.promTarget = u.Host

	return counter
}

func TestQueryDoesNotLeakGoroutines(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer server.Close()

	counter := newMockMetricsClient(t, server)

	runtime.GC()
	before := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		_, err := counter.Query("up")
		if err != nil {
			t.Fatalf("Query call %d failed: %v", i, err)
		}
	}

	runtime.GC()
	after := runtime.NumGoroutine()
	leaked := after - before

	if leaked > 10 {
		t.Errorf("goroutine leak: %d goroutines before, %d after (%d leaked) — "+
			"http.Transport created per-request in RoundTrip is not being closed", before, after, leaked)
	}
}

func TestQueryReturnsResults(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query().Get("query")
		if q != "up" {
			t.Errorf("unexpected query param: %s", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"metric":{"alertname":"TestAlert"},"value":[1,"1"]}]}}`))
	}))
	defer server.Close()

	counter := newMockMetricsClient(t, server)

	resp, err := counter.Query("up")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(resp.Data.Result) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Data.Result))
	}
	if resp.Data.Result[0].Metric["alertname"] != "TestAlert" {
		t.Errorf("unexpected alertname: %s", resp.Data.Result[0].Metric["alertname"])
	}
}

func TestQuerySetsAuthHeader(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("unexpected Authorization header: %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer server.Close()

	counter := newMockMetricsClient(t, server)

	_, err := counter.Query("up")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
}

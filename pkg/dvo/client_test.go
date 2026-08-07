package dvo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"testing"

	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

func newMockDvoClient(t *testing.T, server *httptest.Server) DvoClient {
	t.Helper()

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)
	mockClient := mocks.NewMockClient(mockCtrl)

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	svcPort := int32(port) //nolint:gosec // test port from httptest, no overflow risk

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
	if err != nil {
		t.Fatalf("failed to build DVO client: %v", err)
	}

	realClient, ok := dc.(*dvoClient)
	if !ok {
		t.Fatal("unexpected DvoClient implementation type")
	}
	realClient.dvoBaseUrl = u.Host

	return dc
}

func TestGetMetricsDoesNotLeakGoroutines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# HELP some_metric\nsome_metric 1\n"))
	}))
	defer server.Close()

	dc := newMockDvoClient(t, server)

	runtime.GC()
	before := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		_, err := dc.GetMetrics()
		if err != nil {
			t.Fatalf("GetMetrics call %d failed: %v", i, err)
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

func TestGetMetricsReturnsBody(t *testing.T) {
	expected := "# HELP some_metric\nsome_metric 1\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expected))
	}))
	defer server.Close()

	dc := newMockDvoClient(t, server)

	body, err := dc.GetMetrics()
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
	if string(body) != expected {
		t.Errorf("got %q, want %q", string(body), expected)
	}
}

func TestGetMetricsSetsAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("missing Authorization header")
		}
		if auth != "AccessToken test-cluster:test-pull-secret" {
			t.Errorf("unexpected Authorization header: %s", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dc := newMockDvoClient(t, server)

	_, err := dc.GetMetrics()
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
}

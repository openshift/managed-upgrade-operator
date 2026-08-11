package dvo

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DVO Client", func() {
	var (
		testServer *httptest.Server
		client     *dvoClient
	)

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("GetMetrics", func() {
		It("returns metrics on a successful response", func() {
			expectedBody := `# HELP deployment_validation_operator_total Total deployments checked
# TYPE deployment_validation_operator_total counter
deployment_validation_operator_total 42
`
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal(METRICS_API_PATH))
				Expect(r.Method).To(Equal(http.MethodGet))
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, expectedBody)
			}))

			client = &dvoClient{
				dvoBaseUrl: testServer.Listener.Addr().String(),
				httpClient: *testServer.Client(),
			}

			body, err := client.GetMetrics()
			Expect(err).To(BeNil())
			Expect(string(body)).To(Equal(expectedBody))
		})

		It("does not send an Authorization header", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(BeEmpty())
				w.WriteHeader(http.StatusOK)
			}))

			client = &dvoClient{
				dvoBaseUrl: testServer.Listener.Addr().String(),
				httpClient: *testServer.Client(),
			}

			_, err := client.GetMetrics()
			Expect(err).To(BeNil())
		})

		It("returns an error when the server is unreachable", func() {
			client = &dvoClient{
				dvoBaseUrl: "127.0.0.1:1",
				httpClient: http.Client{Transport: newDvoTransport()},
			}

			_, err := client.GetMetrics()
			Expect(err).ToNot(BeNil())
		})

		It("returns the body even on non-200 status codes", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "internal error")
			}))

			client = &dvoClient{
				dvoBaseUrl: testServer.Listener.Addr().String(),
				httpClient: *testServer.Client(),
			}

			body, err := client.GetMetrics()
			Expect(err).To(BeNil())
			Expect(string(body)).To(Equal("internal error"))
		})
	})

	Context("newDvoTransport", func() {
		It("returns a transport with proxy support configured", func() {
			transport := newDvoTransport()
			Expect(transport).ToNot(BeNil())
			Expect(transport.Proxy).ToNot(BeNil())
			Expect(transport.TLSHandshakeTimeout.Seconds()).To(Equal(float64(30)))
		})
	})
})

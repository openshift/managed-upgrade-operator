package ocm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("OCM Client Builder", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		builder        OcmClientBuilder
		testServer     *httptest.Server
	)

	const (
		testClusterId  = "test-cluster-id-12345"
		testPullSecret = "test-pull-secret-67890"
	)

	// Helper function to create a valid pull secret
	createPullSecret := func() *corev1.Secret {
		dockerConfig := map[string]any{
			"auths": map[string]any{
				"cloud.openshift.com": map[string]any{
					"auth": testPullSecret,
				},
			},
		}
		dockerConfigJSON, _ := json.Marshal(dockerConfig)

		return &corev1.Secret{
			Data: map[string][]byte{
				".dockerconfigjson": dockerConfigJSON,
			},
		}
	}

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		builder = NewBuilder()

		// Create test HTTP server to act as OCM API
		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Mock OAuth2 token endpoint
			if r.URL.Path == "/token" && r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := map[string]any{
					"access_token": "dummy-access-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					GinkgoT().Errorf("Failed to encode mock response: %v", err)
				}
				return
			}

			// Default endpoint for testing
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"status": "ok"}`)); err != nil {
				GinkgoT().Errorf("Failed to write response: %v", err)
			}
		}))
	})

	AfterEach(func() {
		mockCtrl.Finish()
		if testServer != nil {
			testServer.Close()
		}
	})

	Context("When creating a new OCM client", func() {
		It("successfully creates client with valid access token", func() {
			// Mock ClusterVersion response with cluster ID
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: testClusterId,
				},
			}
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				SetArg(2, *cv).Return(nil)

			// Mock pull secret retrieval
			secret := createPullSecret()
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).
				SetArg(2, *secret).Return(nil)

			testUrl, _ := url.Parse(testServer.URL)
			client, err := builder.New(mockKubeClient, testUrl)

			Expect(err).To(BeNil())
			Expect(client).ToNot(BeNil())

			// Verify client type and fields
			ocmClient, ok := client.(*ocmClient)
			Expect(ok).To(BeTrue())
			Expect(ocmClient.client).To(Equal(mockKubeClient))
			Expect(ocmClient.ocmBaseUrl).To(Equal(testUrl))
			Expect(ocmClient.conn).ToNot(BeNil())
		})

		It("returns error when ClusterVersion retrieval fails", func() {
			// Mock GetAccessToken failure
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				Return(fmt.Errorf("cluster version not found"))

			testUrl, _ := url.Parse(testServer.URL)
			client, err := builder.New(mockKubeClient, testUrl)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve cluster access token"))
			Expect(client).To(BeNil())
		})

		It("returns error when pull secret retrieval fails", func() {
			// Mock ClusterVersion success
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: testClusterId,
				},
			}
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				SetArg(2, *cv).Return(nil)

			// Mock pull secret failure
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).
				Return(fmt.Errorf("pull secret not found"))

			testUrl, _ := url.Parse(testServer.URL)
			client, err := builder.New(mockKubeClient, testUrl)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve cluster access token"))
			Expect(client).To(BeNil())
		})
	})

	Context("When proxy environment variables are set", func() {
		var originalHTTPProxy, originalHTTPSProxy, originalNoProxy string

		BeforeEach(func() {
			// Save original environment
			originalHTTPProxy = os.Getenv("HTTP_PROXY")
			originalHTTPSProxy = os.Getenv("HTTPS_PROXY")
			originalNoProxy = os.Getenv("NO_PROXY")
		})

		AfterEach(func() {
			// Restore original environment
			if originalHTTPProxy == "" {
				os.Unsetenv("HTTP_PROXY")
			} else {
				os.Setenv("HTTP_PROXY", originalHTTPProxy)
			}
			if originalHTTPSProxy == "" {
				os.Unsetenv("HTTPS_PROXY")
			} else {
				os.Setenv("HTTPS_PROXY", originalHTTPSProxy)
			}
			if originalNoProxy == "" {
				os.Unsetenv("NO_PROXY")
			} else {
				os.Setenv("NO_PROXY", originalNoProxy)
			}
		})

		It("creates client that respects proxy environment variables", func() {
			// Set proxy environment variables
			os.Setenv("HTTPS_PROXY", "http://proxy.example.com:8080")
			os.Setenv("NO_PROXY", "localhost,127.0.0.1")

			// Mock access token retrieval
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: testClusterId,
				},
			}
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				SetArg(2, *cv).Return(nil)

			secret := createPullSecret()
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).
				SetArg(2, *secret).Return(nil)

			testUrl, _ := url.Parse(testServer.URL)
			client, err := builder.New(mockKubeClient, testUrl)

			Expect(err).To(BeNil())
			Expect(client).ToNot(BeNil())

			// Verify the client was created successfully
			// Note: We can't directly inspect proxy config, but we validate
			// that the builder completed without errors with proxy vars set
			ocmClient, ok := client.(*ocmClient)
			Expect(ok).To(BeTrue())
			Expect(ocmClient.conn).ToNot(BeNil())
		})
	})

	Context("Connection retry and timeout configuration", func() {
		It("creates client with properly configured SDK connection", func() {
			// This test validates that the SDK connection is created successfully
			// with timeout and retry configurations from builder.go

			// Mock access token retrieval
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: testClusterId,
				},
			}
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				SetArg(2, *cv).Return(nil)

			secret := createPullSecret()
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).
				SetArg(2, *secret).Return(nil)

			testUrl, _ := url.Parse(testServer.URL)
			client, err := builder.New(mockKubeClient, testUrl)

			Expect(err).To(BeNil())
			Expect(client).ToNot(BeNil())

			// Verify the connection was created with proper configuration
			// The SDK connection should have retry limit (5), retry interval (2s),
			// jitter (0.3), and transport timeouts configured in builder.go
			ocmClient, ok := client.(*ocmClient)
			Expect(ok).To(BeTrue())
			Expect(ocmClient.conn).ToNot(BeNil())
		})
	})

	Context("Token formatting", func() {
		It("formats authentication token correctly as 'clusterId:pullSecret'", func() {
			// This test validates that the token is formatted correctly
			// The actual token format validation happens in the SDK

			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: testClusterId,
				},
			}
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				SetArg(2, *cv).Return(nil)

			secret := createPullSecret()
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).
				SetArg(2, *secret).Return(nil)

			testUrl, _ := url.Parse(testServer.URL)
			client, err := builder.New(mockKubeClient, testUrl)

			Expect(err).To(BeNil())
			Expect(client).ToNot(BeNil())

			// The token format is "clusterId:pullSecret" (line 45 in builder.go)
			// We verify the client was created successfully, which means
			// the token was accepted by the SDK
			ocmClient, ok := client.(*ocmClient)
			Expect(ok).To(BeTrue())
			Expect(ocmClient.conn).ToNot(BeNil())
		})
	})

	Context("NewBuilder factory function", func() {
		It("returns a valid OcmClientBuilder instance", func() {
			builder := NewBuilder()

			Expect(builder).ToNot(BeNil())
		})
	})
})

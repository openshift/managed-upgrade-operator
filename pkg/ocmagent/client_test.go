package ocmagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	sdk "github.com/openshift-online/ocm-sdk-go"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const (
	TEST_CLUSTER_ID                 = "111111-2222222-3333333-4444444"
	TEST_POLICY_ID_MANUAL           = "aaaaaa-bbbbbb-cccccc-dddddd"
	TEST_UPGRADEPOLICY_UPGRADETYPE  = "OSD"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60
	TEST_VALUE                      = "scheduled"
	TEST_DESCRIPTION                = "Upgrade scheduled"
)

var _ = Describe("OCM Agent Client with SDK", func() {
	var (
		mockCtrl   *gomock.Controller
		testServer *httptest.Server
		conn       *sdk.Connection
		oc         ocmClient
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		// Create test HTTP server that mimics ocm-agent responses
		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set(OPERATION_ID_HEADER, "test-operation-id")

			switch {
			case r.URL.Path == "/" && r.Method == http.MethodGet:
				// Return cluster info (ocm-agent root endpoint)
				response := map[string]interface{}{
					"id": TEST_CLUSTER_ID,
					"version": map[string]interface{}{
						"id":            "4.4.4",
						"channel_group": TEST_UPGRADEPOLICY_CHANNELGROUP,
					},
					"node_drain_grace_period": map[string]interface{}{
						"value": TEST_UPGRADEPOLICY_PDB_TIME,
						"unit":  "minutes",
					},
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					GinkgoT().Errorf("Failed to encode mock response: %v", err)
				}

			case r.URL.Path == fmt.Sprintf("/api/clusters_mgmt/v1/clusters/%s/upgrade_policies", TEST_CLUSTER_ID) && r.Method == http.MethodGet:
				// Return upgrade policies list
				response := map[string]interface{}{
					"kind":  "UpgradePolicyList",
					"page":  1,
					"size":  1,
					"total": 1,
					"items": []map[string]interface{}{
						{
							"id":            TEST_POLICY_ID_MANUAL,
							"schedule_type": "manual",
							"upgrade_type":  TEST_UPGRADEPOLICY_UPGRADETYPE,
							"version":       TEST_UPGRADEPOLICY_VERSION,
							"cluster_id":    TEST_CLUSTER_ID,
						},
					},
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					GinkgoT().Errorf("Failed to encode mock response: %v", err)
				}

			case r.URL.Path == fmt.Sprintf("/api/clusters_mgmt/v1/clusters/%s/upgrade_policies/%s/state", TEST_CLUSTER_ID, TEST_POLICY_ID_MANUAL) && r.Method == http.MethodGet:
				// Return upgrade policy state
				response := map[string]interface{}{
					"kind":        "UpgradePolicyState",
					"value":       "scheduled",
					"description": "Upgrade is scheduled",
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					GinkgoT().Errorf("Failed to encode mock response: %v", err)
				}

			case r.URL.Path == fmt.Sprintf("/api/clusters_mgmt/v1/clusters/%s/upgrade_policies/%s/state", TEST_CLUSTER_ID, TEST_POLICY_ID_MANUAL) && r.Method == http.MethodPatch:
				// Update state
				w.WriteHeader(http.StatusOK)
				response := map[string]interface{}{
					"kind":        "UpgradePolicyState",
					"value":       TEST_VALUE,
					"description": TEST_DESCRIPTION,
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					GinkgoT().Errorf("Failed to encode mock response: %v", err)
				}

			case r.URL.Path == "/token" && r.Method == http.MethodPost:
				// Mock OAuth2 token endpoint - return a properly formatted JWT
				w.WriteHeader(http.StatusOK)
				response := map[string]interface{}{
					"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IlRlc3QgVXNlciIsImlhdCI6MTUxNjIzOTAyMn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
					"token_type":   "Bearer",
					"expires_in":   3600,
				}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					GinkgoT().Errorf("Failed to encode mock response: %v", err)
				}

			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		// Create SDK connection pointing to test server
		var err error
		conn, err = sdk.NewConnectionBuilder().
			URL(testServer.URL).
			TokenURL(testServer.URL + "/token"). // Point to test server for token refresh
			Tokens("test-token").                 // Add test token for authentication
			Insecure(true).                       // Skip TLS verification for test server
			Build()
		Expect(err).To(BeNil())

		ocmServerUrl, _ := url.Parse(testServer.URL)
		oc = ocmClient{
			ocmBaseUrl: ocmServerUrl,
			conn:       conn,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		if testServer != nil {
			testServer.Close()
		}
		if conn != nil {
			conn.Close()
		}
	})

	Context("When getting cluster info from ocm-agent", func() {
		It("returns the correct SDK cluster type", func() {
			result, err := oc.GetCluster()
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			Expect(result.ID()).To(Equal(TEST_CLUSTER_ID))
			Expect(result.Version().ChannelGroup()).To(Equal(TEST_UPGRADEPOLICY_CHANNELGROUP))
			Expect(result.NodeDrainGracePeriod().Value()).To(Equal(float64(TEST_UPGRADEPOLICY_PDB_TIME)))
		})
	})

	Context("When getting upgrade policies via ocm-agent", func() {
		It("returns SDK upgrade policies list response", func() {
			result, err := oc.GetClusterUpgradePolicies(TEST_CLUSTER_ID)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			Expect(result.Total()).To(Equal(1))
			Expect(result.Items().Len()).To(Equal(1))

			policy := result.Items().Get(0)
			Expect(policy.ID()).To(Equal(TEST_POLICY_ID_MANUAL))
			Expect(policy.Version()).To(Equal(TEST_UPGRADEPOLICY_VERSION))
		})
	})

	Context("When getting upgrade policy state via ocm-agent", func() {
		It("returns SDK upgrade policy state", func() {
			result, err := oc.GetClusterUpgradePolicyState(TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			Expect(string(result.Value())).To(Equal("scheduled"))
			Expect(result.Description()).To(Equal("Upgrade is scheduled"))
		})
	})

	Context("When setting policy state via ocm-agent", func() {
		It("updates the state successfully", func() {
			err := oc.SetState(TEST_VALUE, TEST_DESCRIPTION, TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID)
			Expect(err).To(BeNil())
		})
	})
})

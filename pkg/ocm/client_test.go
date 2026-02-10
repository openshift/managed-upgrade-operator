package ocm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const (
	TEST_CLUSTER_ID         = "111111-2222222-3333333-4444444"
	TEST_EXTERNAL_ID        = "external-" + TEST_CLUSTER_ID
	TEST_POLICY_ID_MANUAL   = "aaaaaa-bbbbbb-cccccc-dddddd"
	TEST_OPERATOR_NAMESPACE = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE  = "OSD"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60
	TEST_VALUE                      = "scheduled"
	TEST_DESCRIPTION                = "New Upgrade"
)

var _ = Describe("OCM Client with SDK", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		testServer     *httptest.Server
		conn           *sdk.Connection
		oc             ocmClient
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)

		// Create test HTTP server that mimics OCM API
		testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set(OPERATION_ID_HEADER, "test-operation-id")

			switch {
			case r.URL.Path == "/api/clusters_mgmt/v1/clusters" && r.Method == http.MethodGet:
				// Return cluster list response
				response := map[string]interface{}{
					"kind":  "ClusterList",
					"page":  1,
					"size":  1,
					"total": 1,
					"items": []map[string]interface{}{
						{
							"id": TEST_CLUSTER_ID,
							"external_id": TEST_EXTERNAL_ID,
							"version": map[string]interface{}{
								"id":            "4.4.4",
								"channel_group": TEST_UPGRADEPOLICY_CHANNELGROUP,
							},
							"node_drain_grace_period": map[string]interface{}{
								"value": float64(TEST_UPGRADEPOLICY_PDB_TIME),
								"unit":  "minutes",
							},
						},
					},
				}
				json.NewEncoder(w).Encode(response)

			case r.URL.Path == fmt.Sprintf("/api/clusters_mgmt/v1/clusters/%s/upgrade_policies", TEST_CLUSTER_ID) && r.Method == http.MethodGet:
				// Return upgrade policies list
				nextRun, _ := time.Parse(time.RFC3339, "2020-06-20T00:00:00Z")
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
							"next_run":      nextRun.Format(time.RFC3339),
							"cluster_id":    TEST_CLUSTER_ID,
						},
					},
				}
				json.NewEncoder(w).Encode(response)

			case r.URL.Path == fmt.Sprintf("/api/clusters_mgmt/v1/clusters/%s/upgrade_policies/%s/state", TEST_CLUSTER_ID, TEST_POLICY_ID_MANUAL) && r.Method == http.MethodGet:
				// Return upgrade policy state
				response := map[string]interface{}{
					"kind":        "UpgradePolicyState",
					"value":       "scheduled",
					"description": "Upgrade is scheduled",
				}
				json.NewEncoder(w).Encode(response)

			case r.URL.Path == fmt.Sprintf("/api/clusters_mgmt/v1/clusters/%s/upgrade_policies/%s/state", TEST_CLUSTER_ID, TEST_POLICY_ID_MANUAL) && r.Method == http.MethodPatch:
				// Update state
				w.WriteHeader(http.StatusOK)
				response := map[string]interface{}{
					"kind":        "UpgradePolicyState",
					"value":       TEST_VALUE,
					"description": TEST_DESCRIPTION,
				}
				json.NewEncoder(w).Encode(response)

			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		// Create SDK connection pointing to test server
		var err error
		conn, err = sdk.NewConnectionBuilder().
			URL(testServer.URL).
			Tokens("test-token").  // Add test token for authentication
			Insecure(true).        // Skip TLS verification for test server
			Build()
		Expect(err).To(BeNil())

		ocmServerUrl, _ := url.Parse(testServer.URL)
		oc = ocmClient{
			client:     mockKubeClient,
			ocmBaseUrl: ocmServerUrl,
			conn:       conn,
		}

		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
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

	Context("When getting cluster info", func() {
		It("returns the correct SDK cluster type", func() {
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_EXTERNAL_ID,
				},
			}

			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).
				SetArg(2, *cv).Return(nil)

			result, err := oc.GetCluster()
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			Expect(result.ID()).To(Equal(TEST_CLUSTER_ID))
			Expect(result.Version().ChannelGroup()).To(Equal(TEST_UPGRADEPOLICY_CHANNELGROUP))
			Expect(result.NodeDrainGracePeriod().Value()).To(Equal(float64(TEST_UPGRADEPOLICY_PDB_TIME)))
		})
	})

	Context("When getting upgrade policies", func() {
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

	Context("When getting upgrade policy state", func() {
		It("returns SDK upgrade policy state", func() {
			result, err := oc.GetClusterUpgradePolicyState(TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			Expect(string(result.Value())).To(Equal("scheduled"))
			Expect(result.Description()).To(Equal("Upgrade is scheduled"))
		})
	})

	Context("When setting policy state", func() {
		It("updates the state successfully", func() {
			err := oc.SetState(TEST_VALUE, TEST_DESCRIPTION, TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID)
			Expect(err).To(BeNil())
		})
	})
})

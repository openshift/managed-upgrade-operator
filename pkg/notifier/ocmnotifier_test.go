package notifier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/types"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	mockUCMgr "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_CLUSTER_ID                  = "111111-2222222-3333333-4444444"
	TEST_POLICY_ID                   = "aaaaaa-bbbbbb-cccccc-dddddd"
	TEST_CLUSTER_ID_WITH_NO_POLICIES = "555555-666666-777777-888888"
	TEST_CLUSTER_ID_FOR_BAD_REPLY    = "999999-aaaaaa-bbbbbb-cccccc"
	TEST_CLUSTER_ID_FOR_SAME_STATE   = "000000-000000-000000-000000"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE        = "openshift-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE = "OSD"
	TEST_UPGRADEPOLICY_TIME        = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION     = "4.4.5"

	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60

	// State constants
	TEST_STATE_VALUE       = "test-value"
	TEST_STATE_DESCRIPTION = "test-description"
)

var _ = Describe("OCM Notifier", func() {
	var (
		mockCtrl                 *gomock.Controller
		mockKubeClient           *mocks.MockClient
		mockUpgradeConfigManager *mockUCMgr.MockUpgradeConfigManager
		notifier                 *ocmNotifier
		ocmServer                *httptest.Server
		upgradeConfigName        types.NamespacedName
	)

	BeforeEach(func() {
		ocmServer = ocmServerMock()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockUpgradeConfigManager = mockUCMgr.NewMockUpgradeConfigManager(mockCtrl)

		ocmServerUrl, _ := url.Parse(ocmServer.URL)

		notifier = &ocmNotifier{
			clusterID:            TEST_CLUSTER_ID,
			client:               mockKubeClient,
			ocmBaseUrl:           ocmServerUrl,
			httpClient:           &http.Client{},
			upgradeConfigManager: mockUpgradeConfigManager,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		ocmServer.Close()
	})

	Context("Notify state", func() {
		BeforeEach(func() {
			ns := TEST_OPERATOR_NAMESPACE
			upgradeConfigName = types.NamespacedName{
				Name:      "test-upgradeconfig",
				Namespace: ns,
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", ns)
			notifier.clusterID = TEST_CLUSTER_ID
		})

		Context("When an associated policy ID can't be found", func() {
			var uc upgradev1alpha1.UpgradeConfig
			BeforeEach(func() {
				uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
				uc.Spec.Desired.Version = TEST_UPGRADEPOLICY_VERSION
				uc.Spec.UpgradeAt = TEST_UPGRADEPOLICY_TIME
				notifier.clusterID = TEST_CLUSTER_ID_WITH_NO_POLICIES
			})
			It("returns an error", func() {
				mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil)
				err := notifier.NotifyState(TEST_STATE_VALUE, TEST_STATE_DESCRIPTION)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("can't determine policy ID"))
			})
		})

		Context("When a policy exists but doesn't match version", func() {
			var uc upgradev1alpha1.UpgradeConfig
			BeforeEach(func() {
				uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
				uc.Spec.Desired.Version = "not the same version"
				uc.Spec.UpgradeAt = TEST_UPGRADEPOLICY_TIME
				notifier.clusterID = TEST_CLUSTER_ID
			})
			It("returns an error", func() {
				mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil)
				err := notifier.NotifyState(TEST_STATE_VALUE, TEST_STATE_DESCRIPTION)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("can't determine policy ID"))
			})
		})

		Context("When a policy exists but doesn't match upgrade time", func() {
			var uc upgradev1alpha1.UpgradeConfig
			BeforeEach(func() {
				uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
				uc.Spec.Desired.Version = TEST_UPGRADEPOLICY_VERSION
				uc.Spec.UpgradeAt = "not the same time"
				notifier.clusterID = TEST_CLUSTER_ID
			})
			It("returns an error", func() {
				mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil)
				err := notifier.NotifyState(TEST_STATE_VALUE, TEST_STATE_DESCRIPTION)
				Expect(err).NotTo(BeNil())
				Expect(err.Error()).To(ContainSubstring("can't determine policy ID"))
			})
		})

		Context("When a matching policy can be found", func() {

			Context("When the policy state matches the notifying state", func() {
				var uc upgradev1alpha1.UpgradeConfig
				BeforeEach(func() {
					uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
					uc.Spec.Desired.Version = TEST_UPGRADEPOLICY_VERSION
					uc.Spec.UpgradeAt = TEST_UPGRADEPOLICY_TIME
					notifier.clusterID = TEST_CLUSTER_ID_FOR_SAME_STATE
				})
				It("does not send a notification", func() {
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil)
					err := notifier.NotifyState(TEST_STATE_VALUE, TEST_STATE_DESCRIPTION)
					Expect(err).To(BeNil())
				})

			})

			Context("When the policy state does not match the notifying state", func() {
				var uc upgradev1alpha1.UpgradeConfig
				BeforeEach(func() {
					uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
					uc.Spec.Desired.Version = TEST_UPGRADEPOLICY_VERSION
					uc.Spec.UpgradeAt = TEST_UPGRADEPOLICY_TIME
					notifier.clusterID = TEST_CLUSTER_ID
				})
				It("sends a notification", func() {
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil)
					err := notifier.NotifyState(TEST_STATE_VALUE, TEST_STATE_DESCRIPTION)
					Expect(err).To(BeNil())
				})
			})

		})

	})
})

func ocmServerMock() *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc(CLUSTERS_V1_PATH, clustersMock)

	upPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(upPolicyPath, policyMock)

	noPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_WITH_NO_POLICIES, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(noPolicyPath, noPolicyMock)

	statePath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH, TEST_POLICY_ID, STATE_V1_PATH)
	handler.HandleFunc(statePath, stateMock)

	upPolicyPathSameState := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_FOR_SAME_STATE, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(upPolicyPathSameState, policyMockSameState)
	statePathSameState := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_FOR_SAME_STATE, UPGRADEPOLICIES_V1_PATH, TEST_POLICY_ID, STATE_V1_PATH)
	handler.HandleFunc(statePathSameState, stateMockSameValue)

	srv := httptest.NewServer(handler)
	return srv
}

func clustersMock(w http.ResponseWriter, r *http.Request) {
	response := clusterList{
		Kind:  "ClusterList",
		Page:  1,
		Size:  1,
		Total: 1,
		Items: []clusterInfo{
			{
				Id: "",
				Version: clusterVersion{
					Id:           "4.4.4",
					ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
				},
			},
		},
	}

	// Return different responses based on the cluster ID searched for
	clusterSearch := r.URL.Query().Get("search")

	// Return a cluster ID that'll have some upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID) {
		response.Items[0].Id = TEST_CLUSTER_ID
	}
	// Return a cluster ID that'll have no upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID_WITH_NO_POLICIES) {
		response.Items[0].Id = TEST_CLUSTER_ID_WITH_NO_POLICIES
	}
	// Return a cluster ID that'll have no upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID_FOR_BAD_REPLY) {
		response.Items[0].Id = TEST_CLUSTER_ID_FOR_BAD_REPLY
	}

	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func policyMock(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyList{
		Kind:  "UpgradePolicyList",
		Page:  1,
		Size:  1,
		Total: 1,
		Items: []upgradePolicy{
			{
				Id:           TEST_POLICY_ID,
				Kind:         "UpgradePolicy",
				Href:         "test",
				Schedule:     "test",
				ScheduleType: "manual",
				UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
				Version:      TEST_UPGRADEPOLICY_VERSION,
				NextRun:      TEST_UPGRADEPOLICY_TIME,
				NodeDrainGracePeriod: nodeDrainGracePeriod{
					Value: TEST_UPGRADEPOLICY_PDB_TIME,
					Unit:  "minutes",
				},
				ClusterId: TEST_CLUSTER_ID,
			},
		},
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func noPolicyMock(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyList{
		Kind:  "UpgradePolicyList",
		Page:  1,
		Size:  0,
		Total: 0,
		Items: []upgradePolicy{},
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func policyMockSameState(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyList{
		Kind:  "UpgradePolicyList",
		Page:  1,
		Size:  1,
		Total: 1,
		Items: []upgradePolicy{
			{
				Id:           TEST_POLICY_ID,
				Kind:         "UpgradePolicy",
				Href:         "test",
				Schedule:     "test",
				ScheduleType: "manual",
				UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
				Version:      TEST_UPGRADEPOLICY_VERSION,
				NextRun:      TEST_UPGRADEPOLICY_TIME,
				NodeDrainGracePeriod: nodeDrainGracePeriod{
					Value: TEST_UPGRADEPOLICY_PDB_TIME,
					Unit:  "minutes",
				},
				ClusterId: TEST_CLUSTER_ID_FOR_SAME_STATE,
			},
		},
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func stateMockSameValue(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyState{
		Kind:        "UpgradePolicyState",
		Href:        "test",
		Value:       "test",
		Description: "test",
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func stateMock(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyState{
		Kind:        "UpgradePolicyState",
		Href:        "test",
		Value:       TEST_STATE_VALUE,
		Description: TEST_STATE_DESCRIPTION,
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

package notifier

import (
	"fmt"
	"os"

	"github.com/golang/mock/gomock"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"k8s.io/apimachinery/pkg/types"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	mockOcm "github.com/openshift/managed-upgrade-operator/pkg/ocm/mocks"
	mockUCMgr "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_CLUSTER_ID = "111111-2222222-3333333-4444444"
	TEST_POLICY_ID  = "aaaaaa-bbbbbb-cccccc-dddddd"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE        = "openshift-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE = "OSD"
	TEST_UPGRADEPOLICY_TIME        = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION     = "4.4.5"

	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60

	// State constants
	TEST_STATE_VALUE       = StateStarted
	TEST_STATE_DESCRIPTION = "test-description"
)

var _ = Describe("OCM Notifier", func() {
	var (
		mockCtrl                 *gomock.Controller
		mockKubeClient           *mocks.MockClient
		mockUpgradeConfigManager *mockUCMgr.MockUpgradeConfigManager
		mockOcmClient            *mockOcm.MockOcmClient
		notifier                 *ocmNotifier
		upgradeConfigName        types.NamespacedName
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockOcmClient = mockOcm.NewMockOcmClient(mockCtrl)
		mockUpgradeConfigManager = mockUCMgr.NewMockUpgradeConfigManager(mockCtrl)
		notifier = &ocmNotifier{
			client:               mockKubeClient,
			ocmClient:            mockOcmClient,
			upgradeConfigManager: mockUpgradeConfigManager,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Notify state", func() {

		var (
			cluster                   ocm.ClusterInfo
			upgradePolicyListResponse ocm.UpgradePolicyList
			upgradePolicyState        ocm.UpgradePolicyState
		)

		BeforeEach(func() {
			ns := TEST_OPERATOR_NAMESPACE
			upgradeConfigName = types.NamespacedName{
				Name:      "test-upgradeconfig",
				Namespace: ns,
			}
			cluster = ocm.ClusterInfo{
				Id: TEST_CLUSTER_ID,
				Version: ocm.ClusterVersion{
					Id:           "4.4.4",
					ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
				},
			}
			upgradePolicyListResponse = ocm.UpgradePolicyList{
				Kind:  "UpgradePolicyList",
				Page:  1,
				Size:  1,
				Total: 1,
				Items: []ocm.UpgradePolicy{
					{
						Id:           TEST_POLICY_ID,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "test",
						ScheduleType: "manual",
						UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME,
						ClusterId:    TEST_CLUSTER_ID,
					},
				},
			}
			upgradePolicyState = ocm.UpgradePolicyState{
				Value:       string(TEST_STATE_VALUE),
				Description: TEST_STATE_DESCRIPTION,
			}

			_ = os.Setenv("OPERATOR_NAMESPACE", ns)
		})

		Context("When an associated policy ID can't be found", func() {
			var uc upgradev1alpha1.UpgradeConfig
			BeforeEach(func() {
				uc = *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
				uc.Spec.Desired.Version = TEST_UPGRADEPOLICY_VERSION
				uc.Spec.UpgradeAt = TEST_UPGRADEPOLICY_TIME

			})
			It("returns an error", func() {
				gomock.InOrder(
					mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockOcmClient.EXPECT().GetClusterUpgradePolicies(cluster.Id).Return(nil, fmt.Errorf("fake error")),
				)
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
				upgradePolicyListResponse.Items[0].Version = "different"
			})
			It("returns an error", func() {
				gomock.InOrder(
					mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockOcmClient.EXPECT().GetClusterUpgradePolicies(TEST_CLUSTER_ID).Return(&upgradePolicyListResponse, nil),
				)
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
				upgradePolicyListResponse.Items[0].NextRun = "different"
			})
			It("returns an error", func() {
				gomock.InOrder(
					mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					mockOcmClient.EXPECT().GetClusterUpgradePolicies(TEST_CLUSTER_ID).Return(&upgradePolicyListResponse, nil),
				)
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
				})
				It("does not send a notification", func() {
					gomock.InOrder(
						mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
						mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
						mockOcmClient.EXPECT().GetClusterUpgradePolicies(TEST_CLUSTER_ID).Return(&upgradePolicyListResponse, nil),
						mockOcmClient.EXPECT().GetClusterUpgradePolicyState(TEST_POLICY_ID, TEST_CLUSTER_ID).Return(&upgradePolicyState, nil),
					)
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
				})
				It("sends a notification", func() {
					gomock.InOrder(
						mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
						mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
						mockOcmClient.EXPECT().GetClusterUpgradePolicies(TEST_CLUSTER_ID).Return(&upgradePolicyListResponse, nil),
						mockOcmClient.EXPECT().GetClusterUpgradePolicyState(TEST_POLICY_ID, TEST_CLUSTER_ID).Return(&upgradePolicyState, nil),
						mockOcmClient.EXPECT().SetState(string(StateCompleted), TEST_STATE_DESCRIPTION, TEST_POLICY_ID, TEST_CLUSTER_ID),
					)
					err := notifier.NotifyState(StateCompleted, TEST_STATE_DESCRIPTION)
					Expect(err).To(BeNil())
				})
			})

		})
	})
})

package notifier

import (
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/go-resty/resty/v2"
	"github.com/golang/mock/gomock"
	"github.com/jarcoal/httpmock"
	configv1 "github.com/openshift/api/config/v1"
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

	// OCM test constants
	TEST_OCM_SERVER_URL = "https://fakeapi.openshift.com"
)

var _ = Describe("OCM Notifier", func() {
	var (
		mockCtrl                 *gomock.Controller
		mockKubeClient           *mocks.MockClient
		mockUpgradeConfigManager *mockUCMgr.MockUpgradeConfigManager
		httpClient				 *resty.Client
		notifier                 *ocmNotifier
		upgradeConfigName        types.NamespacedName
	)

	BeforeSuite(func() {
		httpClient = resty.New()
		httpmock.ActivateNonDefault(httpClient.GetClient())
	})

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockUpgradeConfigManager = mockUCMgr.NewMockUpgradeConfigManager(mockCtrl)
		ocmServerUrl, _ := url.Parse(TEST_OCM_SERVER_URL)
		notifier = &ocmNotifier{
			client:               mockKubeClient,
			ocmBaseUrl:           ocmServerUrl,
			httpClient:           httpClient,
			upgradeConfigManager: mockUpgradeConfigManager,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		httpmock.Reset()
	})

	Context("Notify state", func() {

		var (
			clusterListResponse clusterList
			upgradePolicyListResponse upgradePolicyList
		)

		BeforeEach(func() {
			ns := TEST_OPERATOR_NAMESPACE
			upgradeConfigName = types.NamespacedName{
				Name:      "test-upgradeconfig",
				Namespace: ns,
			}
			clusterListResponse = clusterList{
				Kind:  "ClusterList",
				Page:  1,
				Size:  1,
				Total: 1,
				Items: []clusterInfo{
					{
						Id: TEST_CLUSTER_ID,
						Version: clusterVersion{
							Id:           "4.4.4",
							ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
						},
					},
				},
			}
			upgradePolicyListResponse = upgradePolicyList{
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
				clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
				clUrl := path.Join(CLUSTERS_V1_PATH)
				httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

				upResponse := upgradePolicyList{
					Kind:  "UpgradePolicyList",
					Page:  1,
					Size:  0,
					Total: 0,
					Items: []upgradePolicy{},
				}
				upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upResponse)
				upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
				httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{ClusterID: TEST_CLUSTER_ID}}),
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
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
			})
			It("returns an error", func() {
				clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
				clUrl := path.Join(CLUSTERS_V1_PATH)
				httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

				upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
				upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
				httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{ClusterID: TEST_CLUSTER_ID}}),
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
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
			})
			It("returns an error", func() {
				clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
				clUrl := path.Join(CLUSTERS_V1_PATH)
				httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

				upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
				upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
				httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{ClusterID: TEST_CLUSTER_ID}}),
					mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
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
					clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
					clUrl := path.Join(CLUSTERS_V1_PATH)
					httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

					upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
					upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
					httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

					stateResponse := upgradePolicyState{
						Kind:        "UpgradePolicyState",
						Href:        "test",
						Value:       TEST_STATE_VALUE,
						Description: TEST_STATE_DESCRIPTION,
					}
					stateResponder, _ := httpmock.NewJsonResponder(http.StatusOK, stateResponse)
					stateUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH, TEST_POLICY_ID, STATE_V1_PATH)
					httpmock.RegisterResponder(http.MethodGet, stateUrl, stateResponder)

					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{ClusterID: TEST_CLUSTER_ID}}),
						mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
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

					clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
					clUrl := path.Join(CLUSTERS_V1_PATH)
					httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

					upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
					upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
					httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

					stateResponse := upgradePolicyState{
						Kind:        "UpgradePolicyState",
						Href:        "test",
						Value:       "different value",
						Description: "different desc",
					}
					stateResponder, _ := httpmock.NewJsonResponder(http.StatusOK, stateResponse)
					stateUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH, TEST_POLICY_ID, STATE_V1_PATH)
					httpmock.RegisterResponder(http.MethodGet, stateUrl, stateResponder)
					statePatchResponder, _ := httpmock.NewJsonResponder(http.StatusOK, stateResponse)
					httpmock.RegisterResponder(http.MethodPatch,stateUrl, statePatchResponder)

					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{ClusterID: TEST_CLUSTER_ID}}),
						mockUpgradeConfigManager.EXPECT().Get().Return(&uc, nil),
					)
					err := notifier.NotifyState(TEST_STATE_VALUE, TEST_STATE_DESCRIPTION)
					Expect(err).To(BeNil())
				})
			})

		})
	})
})

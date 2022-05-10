package ocmprovider

import (
	"fmt"
	"os"

	"github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	mockOcm "github.com/openshift/managed-upgrade-operator/pkg/ocm/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_CLUSTER_ID                = "111111-2222222-3333333-4444444"
	TEST_CLUSTER_ID_MULTI_POLICIES = "444444-333333-222222-111111"
	TEST_POLICY_ID_MANUAL          = "aaaaaa-bbbbbb-cccccc-dddddd"
	TEST_POLICY_ID_AUTOMATIC       = "aaaaaa-bbbbbb-cccccc-dddddd"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE                 = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE          = "TEST_UPGRADE_TYPE"
	TEST_UPGRADEPOLICY_TIME                 = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING  = "2020-05-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION              = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP         = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME             = 60
	TEST_UPGRADEPOLICY_CAPACITY_RESERVATION = true
)

var _ = Describe("OCM Provider", func() {
	var (
		mockCtrl                   *gomock.Controller
		mockKubeClient             *mocks.MockClient
		mockOcmClient              *mockOcm.MockOcmClient
		provider                   *ocmProvider
		upgradePolicyListResponse  ocm.UpgradePolicyList
		upgradePolicyStateResponse ocm.UpgradePolicyState
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockOcmClient = mockOcm.NewMockOcmClient(mockCtrl)
		provider = &ocmProvider{
			client:      mockKubeClient,
			ocmClient:   mockOcmClient,
			upgradeType: TEST_UPGRADEPOLICY_UPGRADETYPE,
		}
		upgradePolicyListResponse = ocm.UpgradePolicyList{
			Kind:  "UpgradePolicyList",
			Page:  1,
			Size:  1,
			Total: 1,
			Items: []ocm.UpgradePolicy{
				{
					Id:           TEST_POLICY_ID_MANUAL,
					Kind:         "UpgradePolicy",
					Href:         "test",
					Schedule:     "test",
					ScheduleType: "manual",
					Version:      TEST_UPGRADEPOLICY_VERSION,
					NextRun:      TEST_UPGRADEPOLICY_TIME,
					ClusterId:    TEST_CLUSTER_ID,
				},
			},
		}
		upgradePolicyStateResponse = ocm.UpgradePolicyState{
			Kind:        "UpgradePolicyState",
			Href:        "test",
			Value:       "scheduled",
			Description: "Upgrade is scheduled",
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Inferring the upgrade channel", func() {
		It("Sets the channel based on the channel group and version", func() {
			version := "4.9.1"
			channelGroup := "fast"
			channel, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(*channel).To(Equal("fast-4.9"))
			Expect(err).To(BeNil())
		})
		It("Errors if the version is not parseable", func() {
			version := "crashme"
			channelGroup := "fast"
			_, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(err).NotTo(BeNil())
		})

	})

	Context("Getting upgrade policies", func() {
		var (
			cluster                   ocm.ClusterInfo
			upgradePolicyListResponse ocm.UpgradePolicyList
		)

		BeforeEach(func() {
			cluster = ocm.ClusterInfo{

				Id: TEST_CLUSTER_ID,
				Version: ocm.ClusterVersion{
					Id:           "4.4.4",
					ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
				},
				NodeDrainGracePeriod: ocm.NodeDrainGracePeriod{
					Value: TEST_UPGRADEPOLICY_PDB_TIME,
					Unit:  "minutes",
				},
			}

			upgradePolicyListResponse = ocm.UpgradePolicyList{
				Kind:  "UpgradePolicyList",
				Page:  1,
				Size:  1,
				Total: 1,
				Items: []ocm.UpgradePolicy{
					{
						Id:           TEST_POLICY_ID_MANUAL,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "test",
						ScheduleType: "manual",
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME,
						ClusterId:    TEST_CLUSTER_ID,
					},
				},
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		})

		It("Returns specs if they exist", func() {
			expectedSpec := v1alpha1.UpgradeConfigSpec{
				Desired: v1alpha1.Update{
					Version: TEST_UPGRADEPOLICY_VERSION,
					Channel: TEST_UPGRADEPOLICY_CHANNELGROUP + "-4.4",
				},
				UpgradeAt:            TEST_UPGRADEPOLICY_TIME,
				PDBForceDrainTimeout: TEST_UPGRADEPOLICY_PDB_TIME,
				Type:                 TEST_UPGRADEPOLICY_UPGRADETYPE,
				CapacityReservation:  TEST_UPGRADEPOLICY_CAPACITY_RESERVATION,
			}

			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
				mockOcmClient.EXPECT().GetClusterUpgradePolicies(cluster.Id).Return(&upgradePolicyListResponse, nil),
				mockOcmClient.EXPECT().GetClusterUpgradePolicyState(TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID).Return(&upgradePolicyStateResponse, nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(expectedSpec))
		})

		It("Returns next occurring spec if multiple exist", func() {
			expectedSpec := v1alpha1.UpgradeConfigSpec{
				Desired: v1alpha1.Update{
					Version: TEST_UPGRADEPOLICY_VERSION,
					Channel: TEST_UPGRADEPOLICY_CHANNELGROUP + "-4.4",
				},
				UpgradeAt:            TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING,
				PDBForceDrainTimeout: TEST_UPGRADEPOLICY_PDB_TIME,
				Type:                 TEST_UPGRADEPOLICY_UPGRADETYPE,
				CapacityReservation:  TEST_UPGRADEPOLICY_CAPACITY_RESERVATION,
			}

			multiPolicyResponse := ocm.UpgradePolicyList{
				Kind:  "UpgradePolicyList",
				Page:  1,
				Size:  2,
				Total: 2,
				Items: []ocm.UpgradePolicy{
					{
						Id:           TEST_POLICY_ID_MANUAL,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "test",
						ScheduleType: "manual",
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME,
						ClusterId:    TEST_CLUSTER_ID_MULTI_POLICIES,
					},
					{
						Id:           TEST_POLICY_ID_AUTOMATIC,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "3 5 5 * *",
						ScheduleType: "automatic",
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING,
						ClusterId:    TEST_CLUSTER_ID_MULTI_POLICIES,
					},
				},
			}

			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
				mockOcmClient.EXPECT().GetClusterUpgradePolicies(cluster.Id).Return(&multiPolicyResponse, nil),
				mockOcmClient.EXPECT().GetClusterUpgradePolicyState(TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID).Return(&upgradePolicyStateResponse, nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(expectedSpec))
		})

		It("Returns no specs if there are no policies", func() {
			emptyResponse := ocm.UpgradePolicyList{
				Page:  0,
				Size:  0,
				Total: 0,
				Items: []ocm.UpgradePolicy{},
			}
			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
				mockOcmClient.EXPECT().GetClusterUpgradePolicies(cluster.Id).Return(&emptyResponse, nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(BeEmpty())
		})

		It("Errors if the provider is unavailable", func() {
			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(nil, fmt.Errorf("fake error")),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrProviderUnavailable))
			Expect(specs).To(BeNil())
		})

		It("Errors if the internal cluster ID can't be retrieved", func() {

			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(nil, ocm.ErrClusterIdNotFound),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrClusterIdNotFound))
			Expect(specs).To(BeNil())
		})

		It("Errors if the policy response can't be retrieved", func() {
			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(&cluster, nil),
				mockOcmClient.EXPECT().GetClusterUpgradePolicies(cluster.Id).Return(nil, fmt.Errorf("fake error")),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrRetrievingPolicies))
			Expect(specs).To(BeNil())
		})
	})

	Context("Checking if an upgrade policy is actionable", func() {
		It("Will action a normal manual policy", func() {
			u := upgradePolicyListResponse.Items[0]
			result := isActionableUpgradePolicy(&u, &upgradePolicyStateResponse)
			Expect(result).To(BeTrue())
		})
		It("Will not action a policy with no version", func() {
			u := upgradePolicyListResponse.Items[0]
			u.Version = ""
			result := isActionableUpgradePolicy(&u, &upgradePolicyStateResponse)
			Expect(result).To(BeFalse())
		})
		It("Will not action a policy not in a pending state", func() {
			u := upgradePolicyListResponse.Items[0]
			u.Version = ""
			upgradePolicyStateResponse.Value = "somethingelse"
			result := isActionableUpgradePolicy(&u, &upgradePolicyStateResponse)
			Expect(result).To(BeFalse())
		})

	})
})

package localprovider

import (
	"fmt"
	"os"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

const (
	TEST_LOCAL_UPGRADECONFIG_NAME                 = "managed-upgrade-config"
	TEST_LOCAL_UPGRADECONFIG_VERSION              = "4.7.11"
	TEST_LOCAL_UPGRADECONFIG_CHANNELGROUP         = "stable"
	TEST_LOCAL_UPGRADECONFIG_TIME                 = "2021-06-03T00:00:00Z"
	TEST_LOCAL_UPGRADECONFIG_PDB_TIME             = 60
	TEST_LOCAL_UPGRADECONFIG_UPGRADETYPE          = "OSD"
	TEST_LOCAL_UPGRADECONFIG_CAPACITY_RESERVATION = true
	TEST_OPERATOR_NAMESPACE                       = "test-managed-upgrade-operator"
)

var _ = Describe("Local Provider", func() {
	var (
		mockCtrl          *gomock.Controller
		mockKubeClient    *mocks.MockClient
		provider          *localProvider
		upgradeConfigList v1alpha1.UpgradeConfigList
		upgradeConfigSpec v1alpha1.UpgradeConfigSpec
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		provider = &localProvider{
			client:  mockKubeClient,
			cfgname: TEST_LOCAL_UPGRADECONFIG_NAME,
		}

		upgradeConfigSpec = v1alpha1.UpgradeConfigSpec{
			Desired: v1alpha1.Update{
				Version: TEST_LOCAL_UPGRADECONFIG_VERSION,
				Channel: TEST_LOCAL_UPGRADECONFIG_CHANNELGROUP + "-4.7",
			},
			UpgradeAt:            TEST_LOCAL_UPGRADECONFIG_TIME,
			PDBForceDrainTimeout: TEST_LOCAL_UPGRADECONFIG_PDB_TIME,
			Type:                 TEST_LOCAL_UPGRADECONFIG_UPGRADETYPE,
			CapacityReservation:  TEST_LOCAL_UPGRADECONFIG_CAPACITY_RESERVATION,
		}

		upgradeConfigList = v1alpha1.UpgradeConfigList{
			Items: []v1alpha1.UpgradeConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "uc1",
					},
					Spec: upgradeConfigSpec,
					Status: v1alpha1.UpgradeConfigStatus{
						History: []v1alpha1.UpgradeHistory{
							{Version: TEST_LOCAL_UPGRADECONFIG_VERSION, Phase: v1alpha1.UpgradePhasePending},
							{Version: TEST_LOCAL_UPGRADECONFIG_VERSION, Phase: v1alpha1.UpgradePhaseUpgraded},
						},
					},
				},
			},
		}

		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
	})

	Context("Get list of UpgradeConfigSpec", func() {

		It("Returns list of UpgradeConfigSpec", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace(TEST_OPERATOR_NAMESPACE), client.MatchingFields{"metadata.name": TEST_LOCAL_UPGRADECONFIG_NAME},
				}).SetArg(1, upgradeConfigList),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(upgradeConfigSpec))
		})

	})

	Context("Fetch UpgradeConfigList", func() {
		It("Returns UpgradeConfigList if they exist", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace(TEST_OPERATOR_NAMESPACE), client.MatchingFields{"metadata.name": TEST_LOCAL_UPGRADECONFIG_NAME},
				}).SetArg(1, upgradeConfigList),
			)
			_, err := fetchUpgradeConfigs(mockKubeClient)
			Expect(err).To(BeNil())
		})

		It("Errors when failed to return UpgradeConfigList", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace(TEST_OPERATOR_NAMESPACE), client.MatchingFields{"metadata.name": TEST_LOCAL_UPGRADECONFIG_NAME},
				}).SetArg(1, upgradeConfigList).Return(fmt.Errorf("some error")),
			)
			_, err := fetchUpgradeConfigs(mockKubeClient)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Read UpgradeConfigSpec from UpgradeConfigList", func() {
		It("Returns UpgradeConfigList if UpgradeHistory is present", func() {
			specs, err := readSpecFromConfig(upgradeConfigList)
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(upgradeConfigSpec))
		})

		It("Does not return UpgradeConfigSpec when UpgradeHistory is empty", func() {
			emptyHistoryUCL := v1alpha1.UpgradeConfigList{
				Items: []v1alpha1.UpgradeConfig{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "uc1",
						},
						Spec:   upgradeConfigSpec,
						Status: v1alpha1.UpgradeConfigStatus{},
					},
				},
			}
			specs, err := readSpecFromConfig(emptyHistoryUCL)
			Expect(err).To(BeNil())
			Expect(specs).To(BeEmpty())
		})

		It("Does not return UpgradeConfigSpec when UpgradeHistory has Upgraded phase", func() {
			upgradedHistoryUCL := v1alpha1.UpgradeConfigList{
				Items: []v1alpha1.UpgradeConfig{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "uc1",
						},
						Spec: upgradeConfigSpec,
						Status: v1alpha1.UpgradeConfigStatus{
							History: []v1alpha1.UpgradeHistory{
								{Version: TEST_LOCAL_UPGRADECONFIG_VERSION, Phase: v1alpha1.UpgradePhaseUpgraded},
							},
						},
					},
				},
			}
			specs, err := readSpecFromConfig(upgradedHistoryUCL)
			Expect(err).To(BeNil())
			Expect(specs).To(BeEmpty())
		})

	})
})

package upgradeconfigmanager

import (
	"context"
	"fmt"
	"os"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	ppMocks "github.com/openshift/managed-upgrade-operator/pkg/specprovider/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_OPERATOR_NAMESPACE = "test-namespace"
	TEST_UPGRADECONFIG_CR   = "managed-upgrade-config"
	TEST_UPGRADE_VERSION    = "4.4.4"
	TEST_UPGRADE_CHANNEL    = "stable-4.4"
	TEST_UPGRADE_TIME       = "2020-06-20T00:00:00Z"
	TEST_UPGRADE_PDB_TIME   = 60
	TEST_UPGRADE_TYPE       = "OSD"
)

var _ = Describe("UpgradeConfigManager", func() {
	var (
		mockCtrl                 *gomock.Controller
		mockKubeClient           *mocks.MockClient
		manager                  *upgradeConfigManager
		mockConfigManagerBuilder *configMocks.MockConfigManagerBuilder
		mockCVClientBuilder      *cvMocks.MockClusterVersionBuilder
		mockCVClient             *cvMocks.MockClusterVersion
		mockSPClientBuilder      *ppMocks.MockSpecProviderBuilder
		mockSPClient             *ppMocks.MockSpecProvider
	)

	BeforeEach(func() {
		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		mockConfigManagerBuilder = configMocks.NewMockConfigManagerBuilder(mockCtrl)
		mockCVClientBuilder = cvMocks.NewMockClusterVersionBuilder(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		mockSPClientBuilder = ppMocks.NewMockSpecProviderBuilder(mockCtrl)
		mockSPClient = ppMocks.NewMockSpecProvider(mockCtrl)
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
	})

	JustBeforeEach(func() {
		manager = &upgradeConfigManager{
			client:               mockKubeClient,
			cvClientBuilder:      mockCVClientBuilder,
			specProviderBuilder:  mockSPClientBuilder,
			configManagerBuilder: mockConfigManagerBuilder,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Getting the cluster's UpgradeConfigs", func() {
		It("Returns them", func() {
			upgradeConfig := upgradev1alpha1.UpgradeConfig{
				ObjectMeta: v1.ObjectMeta{
					Name:      TEST_UPGRADECONFIG_CR,
					Namespace: TEST_OPERATOR_NAMESPACE,
				},
				Spec: upgradev1alpha1.UpgradeConfigSpec{
					Desired: upgradev1alpha1.Update{
						Version: TEST_UPGRADE_VERSION,
						Channel: TEST_UPGRADE_CHANNEL,
					},
					UpgradeAt:            TEST_UPGRADE_TIME,
					PDBForceDrainTimeout: TEST_UPGRADE_PDB_TIME,
					Type:                 TEST_UPGRADE_TYPE,
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
			)
			uc, err := manager.Get()
			Expect(err).To(BeNil())
			Expect(uc.Spec).To(Equal(upgradeConfig.Spec))
		})

		It("Errors if they can't be retrieved", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("Some error")),
			)
			ucs, err := manager.Get()
			Expect(err).To(Equal(ErrRetrievingUpgradeConfigs))
			Expect(ucs).To(BeNil())
		})
	})

	Context("checking if an upgrade is in progress", func() {
		upgradeConfig := upgradev1alpha1.UpgradeConfig{
			ObjectMeta: v1.ObjectMeta{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			},
			Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{
					Version: TEST_UPGRADE_VERSION,
					Channel: TEST_UPGRADE_CHANNEL,
				},
				UpgradeAt:            TEST_UPGRADE_TIME,
				PDBForceDrainTimeout: TEST_UPGRADE_PDB_TIME,
				Type:                 TEST_UPGRADE_TYPE,
			},
		}

		BeforeEach(func() {
			upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{}
		})

		It("will indicate correctly if UpgradeConfig says so", func() {
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
					Channel:       TEST_UPGRADE_CHANNEL,
				},
				Status: configv1.ClusterVersionStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorProgressing,
							Status: configv1.ConditionTrue,
						},
					},
				},
			}
			upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
				{
					Version: TEST_UPGRADE_VERSION,
					Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
				},
			}
			gomock.InOrder(
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
			)
			inprogress, err := upgradeInProgress(&upgradeConfig, mockCVClient)
			Expect(err).To(BeNil())
			Expect(inprogress).To(BeTrue())
		})

		It("will indicate correctly if UpgradeConfig has no true condition", func() {
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
					Channel:       TEST_UPGRADE_CHANNEL,
				},
				Status: configv1.ClusterVersionStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorProgressing,
							Status: configv1.ConditionTrue,
						},
					},
				},
			}
			upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
				{
					Version: TEST_UPGRADE_VERSION,
					Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
					Conditions: []upgradev1alpha1.UpgradeCondition{
						{
							Type:   upgradev1alpha1.SendStartedNotification,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			gomock.InOrder(
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
			)
			inprogress, err := upgradeInProgress(&upgradeConfig, mockCVClient)
			Expect(err).To(BeNil())
			Expect(inprogress).To(BeTrue())
		})

		It("will indicate correctly if CVO says so", func() {
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
					Channel:       TEST_UPGRADE_CHANNEL,
				},
				Status: configv1.ClusterVersionStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorProgressing,
							Status: configv1.ConditionTrue,
						},
					},
				},
			}
			gomock.InOrder(
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
			)
			inprogress, err := upgradeInProgress(&upgradeConfig, mockCVClient)
			Expect(err).To(BeNil())
			Expect(inprogress).To(BeTrue())
		})

		It("will indicate correctly if neither UpgradeConfig or CVO say so", func() {
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
					Channel:       TEST_UPGRADE_CHANNEL,
				},
				Status: configv1.ClusterVersionStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
					},
				},
			}
			gomock.InOrder(
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
			)
			inprogress, err := upgradeInProgress(&upgradeConfig, mockCVClient)
			Expect(err).To(BeNil())
			Expect(inprogress).To(BeFalse())
		})
	})

	Context("Refreshing UpgradeConfigs", func() {

		upgradeConfig := upgradev1alpha1.UpgradeConfig{
			ObjectMeta: v1.ObjectMeta{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			},
			Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{
					Version: TEST_UPGRADE_VERSION,
					Channel: TEST_UPGRADE_CHANNEL,
				},
				UpgradeAt:            TEST_UPGRADE_TIME,
				PDBForceDrainTimeout: TEST_UPGRADE_PDB_TIME,
				Type:                 TEST_UPGRADE_TYPE,
			},
		}
		cv := &configv1.ClusterVersion{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
				Channel:       TEST_UPGRADE_CHANNEL,
			},
		}

		BeforeEach(func() {
			_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
			upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{}
		})

		It("If existing cluster UpgradeConfigs can't be returned", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(ErrRetrievingUpgradeConfigs),
			)
			_, err := manager.Refresh()
			Expect(err).To(Equal(ErrRetrievingUpgradeConfigs))
		})

		It("should not proceed if an upgrade is occurring", func() {
			cv := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					DesiredUpdate: &configv1.Update{Version: TEST_UPGRADE_VERSION},
					Channel:       TEST_UPGRADE_CHANNEL,
				},
				Status: configv1.ClusterVersionStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorProgressing,
							Status: configv1.ConditionTrue,
						},
					},
				},
			}
			upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
				{
					Version: TEST_UPGRADE_VERSION,
					Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
				mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
			)
			changed, err := manager.Refresh()
			Expect(err).To(BeNil())
			Expect(changed).To(BeFalse())
		})

		It("should indicate if the provider couldn't pull properly", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
				mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
				mockSPClientBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockSPClient, nil),
				mockSPClient.EXPECT().Get().Return(nil, fmt.Errorf("some error")),
			)
			changed, err := manager.Refresh()
			Expect(err).To(Equal(ErrProviderSpecPull))
			Expect(changed).To(BeFalse())
		})

		It("should remove existing UpgradeConfigs if no provider configs are pulled", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
				mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
				mockSPClientBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockSPClient, nil),
				mockSPClient.EXPECT().Get().Return([]upgradev1alpha1.UpgradeConfigSpec{}, nil),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(TEST_UPGRADECONFIG_CR))
						Expect(uc.Namespace).To(Equal(TEST_OPERATOR_NAMESPACE))
						Expect(string(uc.Spec.Type)).To(Equal(TEST_UPGRADE_TYPE))
						Expect(uc.Spec.Desired.Version).To(Equal(TEST_UPGRADE_VERSION))
						Expect(uc.Spec.PDBForceDrainTimeout).To(Equal(int32(TEST_UPGRADE_PDB_TIME)))
						return nil
					}),
			)
			changed, err := manager.Refresh()
			Expect(err).To(BeNil())
			Expect(changed).To(BeTrue())
		})

		It("should indicate no change if no specs are returned by the provider", func() {
			notFound := errors.NewNotFound(schema.GroupResource{
				Group:    "test",
				Resource: "test",
			}, "test")
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound),
				mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
				mockSPClientBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockSPClient, nil),
				mockSPClient.EXPECT().Get().Return([]upgradev1alpha1.UpgradeConfigSpec{}, nil),
			)
			changed, err := manager.Refresh()
			Expect(err).To(BeNil())
			Expect(changed).To(BeFalse())
		})

		It("should create an upgrade config if the provider returns one", func() {
			upgradeConfigSpecs := []upgradev1alpha1.UpgradeConfigSpec{
				upgradeConfig.Spec,
			}
			notFound := errors.NewNotFound(schema.GroupResource{
				Group:    "test",
				Resource: "test",
			}, "test")
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound),
				mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
				mockSPClientBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockSPClient, nil),
				mockSPClient.EXPECT().Get().Return(upgradeConfigSpecs, nil),
				mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(UPGRADECONFIG_CR_NAME))
						Expect(uc.Namespace).To(Equal(TEST_OPERATOR_NAMESPACE))
						Expect(string(uc.Spec.Type)).To(Equal(TEST_UPGRADE_TYPE))
						Expect(uc.Spec.Desired.Version).To(Equal(TEST_UPGRADE_VERSION))
						Expect(uc.Spec.PDBForceDrainTimeout).To(Equal(int32(TEST_UPGRADE_PDB_TIME)))
						return nil
					}),
			)
			changed, err := manager.Refresh()
			Expect(err).To(BeNil())
			Expect(changed).To(BeTrue())
		})

		It("should replace an upgrade config if the provider returns one", func() {
			// the new upgradeconfig to replace with
			upgradeConfigSpecs := []upgradev1alpha1.UpgradeConfigSpec{
				upgradeConfig.Spec,
			}
			// The existing upgradeconfig on the cluster
			oldUpgradeConfig := &upgradev1alpha1.UpgradeConfig{
				ObjectMeta: v1.ObjectMeta{
					Name:      TEST_UPGRADECONFIG_CR,
					Namespace: TEST_OPERATOR_NAMESPACE,
				},
				Spec: upgradev1alpha1.UpgradeConfigSpec{
					Desired: upgradev1alpha1.Update{
						Version: "old version",
						Channel: "old channel",
					},
					UpgradeAt:            "old time",
					PDBForceDrainTimeout: 1,
					Type:                 "old type",
				},
			}
			notFound := errors.NewNotFound(schema.GroupResource{
				Group:    "test",
				Resource: "test",
			}, "test")

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, *oldUpgradeConfig).Return(nil),
				mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
				mockCVClient.EXPECT().GetClusterVersion().Return(cv, nil),
				mockSPClientBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockSPClient, nil),
				mockSPClient.EXPECT().Get().Return(upgradeConfigSpecs, nil),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(notFound),
				mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(TEST_UPGRADECONFIG_CR))
						Expect(uc.Namespace).To(Equal(TEST_OPERATOR_NAMESPACE))
						Expect(string(uc.Spec.Type)).To(Equal(TEST_UPGRADE_TYPE))
						Expect(uc.Spec.Desired.Version).To(Equal(TEST_UPGRADE_VERSION))
						Expect(uc.Spec.PDBForceDrainTimeout).To(Equal(int32(TEST_UPGRADE_PDB_TIME)))
						return nil
					}),
			)
			changed, err := manager.Refresh()
			Expect(err).To(BeNil())
			Expect(changed).To(BeTrue())
		})
	})
})

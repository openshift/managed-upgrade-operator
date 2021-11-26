package upgraders

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("UpgradableCheckStep", func() {
	var (
		logger logr.Logger

		// mocks
		mockKubeClient           *mocks.MockClient
		mockCtrl                 *gomock.Controller
		mockMaintClient          *mockMaintenance.MockMaintenance
		mockScalerClient         *mockScaler.MockScaler
		mockMachineryClient      *mockMachinery.MockMachinery
		mockMetricsClient        *mockMetrics.MockMetrics
		mockCVClient             *cvMocks.MockClusterVersion
		mockDrainStrategyBuilder *mockDrain.MockNodeDrainStrategyBuilder
		mockEMClient             *emMocks.MockEventManager

		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig

		// upgrader to be used during tests
		config   *upgraderConfig
		upgrader *osdUpgrader

		currentClusterVersion *configv1.ClusterVersion
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockMaintClient = mockMaintenance.NewMockMaintenance(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockScalerClient = mockScaler.NewMockScaler(mockCtrl)
		mockMachineryClient = mockMachinery.NewMockMachinery(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		mockDrainStrategyBuilder = mockDrain.NewMockNodeDrainStrategyBuilder(mockCtrl)
		mockEMClient = emMocks.NewMockEventManager(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		config = buildTestUpgraderConfig(90, 30, 8, 120, 30)
		upgrader = &osdUpgrader{
			clusterUpgrader: &clusterUpgrader{
				client:               mockKubeClient,
				metrics:              mockMetricsClient,
				cvClient:             mockCVClient,
				notifier:             mockEMClient,
				config:               config,
				scaler:               mockScalerClient,
				drainstrategyBuilder: mockDrainStrategyBuilder,
				maintenance:          mockMaintClient,
				machinery:            mockMachineryClient,
				upgradeConfig:        upgradeConfig,
			},
		}
		currentClusterVersion = &configv1.ClusterVersion{
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionFalse,
						Reason:  "IsClusterUpgradable not done",
						Message: "Kubernetes 1.22 and therefore OpenShift 4.9 remove several APIs which require admin consideration. Please see the knowledge article https://access.redhat.com/articles/6329921 for details and instructions.",
					},
				},
				History: []configv1.UpdateHistory{
					{
						State: "fakeState",
						StartedTime: v1.Time{
							Time: time.Now().UTC(),
						},
						CompletionTime: &v1.Time{
							Time: time.Now().UTC(),
						},
						Version:  "fakeVersion",
						Verified: false,
					},
				},
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When running the IsUpgradable check", func() {
		Context("When current 'y' stream version is lower then desired version", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = "1.2.3"
				currentClusterVersion.Status.History = []configv1.UpdateHistory{{State: configv1.CompletedUpdate, Version: "1.1.3"}}
			})
			It("will not perform upgrade", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockCVClient.EXPECT().GetClusterVersion().Return(currentClusterVersion, nil),
				)
				result, err := upgrader.IsUpgradeable(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When Upgradeable condition exists and is set to true", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = "1.2.3"
				currentClusterVersion.Status.History = []configv1.UpdateHistory{{State: configv1.CompletedUpdate, Version: "1.1.3"}}
				currentClusterVersion.Status.Conditions = []configv1.ClusterOperatorStatusCondition{{Type: configv1.OperatorUpgradeable, Status: configv1.ConditionTrue}}
			})
			It("will perform upgrade", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockCVClient.EXPECT().GetClusterVersion().Return(currentClusterVersion, nil),
				)
				result, err := upgrader.IsUpgradeable(context.TODO(), logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("When the clusterversion does not have Upgradeable condition", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = "1.2.3"
				currentClusterVersion.Status.History = []configv1.UpdateHistory{{State: configv1.CompletedUpdate, Version: "1.1.3"}}
				currentClusterVersion.Status.Conditions = []configv1.ClusterOperatorStatusCondition{{Type: configv1.OperatorDegraded}}
			})
			It("will perform upgrade", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockCVClient.EXPECT().GetClusterVersion().Return(currentClusterVersion, nil),
				)
				result, err := upgrader.IsUpgradeable(context.TODO(), logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
	})
})

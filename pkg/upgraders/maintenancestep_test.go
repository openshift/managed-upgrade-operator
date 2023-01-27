package upgraders

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("MaintenanceStep", func() {
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
		upgrader *clusterUpgrader
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
		upgrader = &clusterUpgrader{
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
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When removing a control plane maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMaintClient.EXPECT().EndControlPlane()
			result, err := upgrader.RemoveControlPlaneMaintWindow(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			mockMaintClient.EXPECT().EndControlPlane().Return(fmt.Errorf("fake error"))
			result, err := upgrader.RemoveControlPlaneMaintWindow(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Context("When creating a control plane maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMaintClient.EXPECT().StartControlPlane(gomock.Any(), upgradeConfig.Spec.Desired.Version, config.Maintenance.IgnoredAlerts.ControlPlaneCriticals)
			result, err := upgrader.CreateControlPlaneMaintWindow(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			mockMaintClient.EXPECT().StartControlPlane(gomock.Any(), upgradeConfig.Spec.Desired.Version, config.Maintenance.IgnoredAlerts.ControlPlaneCriticals).Return(fmt.Errorf("fake error"))
			result, err := upgrader.CreateControlPlaneMaintWindow(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Context("When creating a worker maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true, MachineCount: 4, UpdatedCount: 2}, nil)
			mockMaintClient.EXPECT().SetWorker(gomock.Any(), upgradeConfig.Spec.Desired.Version, gomock.Any())
			result, err := upgrader.CreateWorkerMaintWindow(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			fakeError := fmt.Errorf("fake error")
			mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true, MachineCount: 4, UpdatedCount: 2}, nil)
			mockMaintClient.EXPECT().SetWorker(gomock.Any(), upgradeConfig.Spec.Desired.Version, gomock.Any()).Return(fakeError)
			result, err := upgrader.CreateWorkerMaintWindow(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
		It("Skip creating maintenance window if no pending worker node left", func() {
			mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true, MachineCount: 4, UpdatedCount: 4}, nil)
			result, err := upgrader.CreateWorkerMaintWindow(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Does not proceed if isUpgrading check fails", func() {
			mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(nil, fmt.Errorf("fake error"))
			result, err := upgrader.CreateWorkerMaintWindow(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
		It("Will not do so if workers are already upgraded", func() {
			mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: false}, nil)
			result, err := upgrader.CreateWorkerMaintWindow(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Context("When removing a worker maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMaintClient.EXPECT().EndWorker()
			result, err := upgrader.RemoveMaintWindow(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			mockMaintClient.EXPECT().EndWorker().Return(fmt.Errorf("fake error"))
			result, err := upgrader.RemoveMaintWindow(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})
})

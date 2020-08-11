package osd_cluster_upgrader

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

var _ = Describe("ClusterUpgrader maintenance window tests", func() {
	var (
		logger            logr.Logger
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		mockKubeClient    *mocks.MockClient
		mockCtrl          *gomock.Controller
		mockMaintClient   *mockMaintenance.MockMaintenance
		mockScaler        *mockScaler.MockScaler
		mockMetricsClient *mockMetrics.MockMetrics
		config            *osdUpgradeConfig
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
		logger = logf.Log.WithName("cluster upgrader test logger")
		stepCounter = make(map[upgradev1alpha1.UpgradeConditionType]int)
		config = &osdUpgradeConfig{
			Maintenance: maintenanceConfig{
				WorkerNodeTime:   8,
				ControlPlaneTime: 90,
				IgnoredAlerts:ignoredAlerts{
					ControlPlaneCriticals: []string{"ignoreAlert1SRE","ignoreAlert2SRE"},
				},
			},
			Scale: scaleConfig{
				TimeOut: 30,
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When removing a control plane maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMaintClient.EXPECT().EndControlPlane()
			result, err := RemoveControlPlaneMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			mockMaintClient.EXPECT().EndControlPlane().Return(fmt.Errorf("fake error"))
			result, err := RemoveControlPlaneMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Context("When creating a control plane maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMaintClient.EXPECT().StartControlPlane(gomock.Any(), upgradeConfig.Spec.Desired.Version, config.Maintenance.IgnoredAlerts.ControlPlaneCriticals)
			result, err := CreateControlPlaneMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			mockMaintClient.EXPECT().StartControlPlane(gomock.Any(), upgradeConfig.Spec.Desired.Version, config.Maintenance.IgnoredAlerts.ControlPlaneCriticals).Return(fmt.Errorf("fake error"))
			result, err := CreateControlPlaneMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})

	Context("When creating a worker maintenance window", func() {
		var configPool *machineconfigapi.MachineConfigPool
		BeforeEach(func() {
			configPool = &machineconfigapi.MachineConfigPool{}
		})
		It("Asks the maintenance client to do so", func() {
			// Set that updated machines lags behind total machines
			configPool.Status.MachineCount = 3
			configPool.Status.UpdatedMachineCount = 1
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "worker"}, gomock.Any()).SetArg(2, *configPool)
			mockMaintClient.EXPECT().SetWorker(gomock.Any(), upgradeConfig.Spec.Desired.Version)
			result, err := CreateWorkerMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			// Set that updated machines lags behind total machines
			configPool.Status.MachineCount = 3
			configPool.Status.UpdatedMachineCount = 1
			fakeError := fmt.Errorf("fake error")
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "worker"}, gomock.Any()).SetArg(2, *configPool)
			mockMaintClient.EXPECT().SetWorker(gomock.Any(), upgradeConfig.Spec.Desired.Version).Return(fakeError)
			result, err := CreateWorkerMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
		It("Does not proceed if workers can't be fetched", func() {
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "worker"}, gomock.Any()).Return(fmt.Errorf("fake error"))
			result, err := CreateWorkerMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
		It("Will not do so if workers are already upgraded", func() {
			// Set that updated machines equals total machines
			configPool.Status.MachineCount = 3
			configPool.Status.UpdatedMachineCount = 3
			mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "worker"}, gomock.Any())
			result, err := CreateWorkerMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Context("When removing a worker maintenance window", func() {
		It("Asks the maintenance client to do so", func() {
			mockMaintClient.EXPECT().EndWorkers()
			result, err := RemoveMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("Indicates when creating the maintenance window has failed", func() {
			mockMaintClient.EXPECT().EndWorkers().Return(fmt.Errorf("fake error"))
			result, err := RemoveMaintWindow(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
	})
})

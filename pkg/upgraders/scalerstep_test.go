package upgraders

import (
	"context"
	"fmt"

	"github.com/openshift/managed-upgrade-operator/pkg/notifier"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("ScalerStep", func() {
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

	Context("Scaling", func() {
		Context("When the scaler says that scaling cannot proceed", func() {
			It("should not attempt to scale", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockScalerClient.EXPECT().CanScale(gomock.Any(), gomock.Any()).Return(false, nil),
					mockEMClient.EXPECT().Notify(notifier.MuoStateScaleSkipped),
				)
				ok, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
		})
		Context("When capacity reservation is enabled", func() {
			It("Should scale up extra nodes and set success metric on successful scaling when capacity reservation enabled", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockScalerClient.EXPECT().CanScale(gomock.Any(), gomock.Any()).Return(true, nil),
					mockScalerClient.EXPECT().EnsureScaleUpNodes(gomock.Any(), config.GetScaleDuration(), gomock.Any()).Return(true, nil),
					mockMetricsClient.EXPECT().UpdateMetricScalingSucceeded(gomock.Any()),
				)

				ok, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
			It("Should set failed metric on scaling time out when capacity reservation enabled", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockScalerClient.EXPECT().CanScale(gomock.Any(), gomock.Any()).Return(true, nil),
					mockScalerClient.EXPECT().EnsureScaleUpNodes(gomock.Any(), config.GetScaleDuration(), gomock.Any()).Return(false, scaler.NewScaleTimeOutError("test scale timed out")),
					mockMetricsClient.EXPECT().UpdateMetricScalingFailed(gomock.Any()),
					mockEMClient.EXPECT().Notify(notifier.MuoStateSkipped),
				)

				ok, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
			It("Should fail when the scale timeout is not set in configmap", func() {
				config.Scale.TimeOut = 0
				ok, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
				Expect(err).NotTo(BeNil())
				Expect(ok).NotTo(BeTrue())
			})
		})

		Context("When capacity reservation is disabled", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.CapacityReservation = false
			})
			It("Should not scale up extra nodes", func() {
				ok, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
			It("Should not scale down extra nodes", func() {
				ok, err := upgrader.RemoveExtraScaledNodes(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
			It("Scale up should not fail if scale timeout is not set in configmap", func() {
				config.Scale.TimeOut = 0
				ok, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
			It("Scale down should not fail if scale timeout is not set in configmap", func() {
				config.Scale.TimeOut = 0
				ok, err := upgrader.RemoveExtraScaledNodes(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("When the cluster's upgrade process has commenced", func() {
		It("will not re-perform spinning up extra workers", func() {
			gomock.InOrder(mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil))
			result, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Context("When the upgrader can't tell if the cluster's upgrade has commenced", func() {
		var fakeError = fmt.Errorf("fake upgradeCommenced error")
		It("will abort the spinning up of extra workers", func() {
			gomock.InOrder(mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, fakeError))
			result, err := upgrader.EnsureExtraUpgradeWorkers(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
	})
})

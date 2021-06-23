package upgraders

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	ac "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks"
	acMocks "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks/mocks"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("HealthCheckStep", func() {
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
		mockAC                   *acMocks.MockAvailabilityChecker
		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig

		config *upgraderConfig

		upgrader *osdUpgrader
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
		mockAC = acMocks.NewMockAvailabilityChecker(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		config = &upgraderConfig{
			Maintenance: maintenanceConfig{
				ControlPlaneTime: 90,
			},
			Scale: scaleConfig{
				TimeOut: 30,
			},
			NodeDrain: drain.NodeDrain{
				ExpectedNodeDrainTime: 8,
			},
			UpgradeWindow: upgradeWindow{
				TimeOut:      120,
				DelayTrigger: 30,
			},
		}
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
				availabilityCheckers: []ac.AvailabilityChecker{mockAC},
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When running the external-dependency-availability-check phase", func() {
		It("return true if all dependencies are available", func() {
			gomock.InOrder(
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
				mockAC.EXPECT().AvailabilityCheck().Return(nil),
			)

			result, err := upgrader.ExternalDependencyAvailabilityCheck(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("return false if any of the dependencies are not available", func() {
			fakeErr := fmt.Errorf("fake error")
			gomock.InOrder(
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
				mockAC.EXPECT().AvailabilityCheck().Return(fakeErr),
			)

			result, err := upgrader.ExternalDependencyAvailabilityCheck(context.TODO(), logger)
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeFalse())
		})
		It("will not perform availability checking if the cluster is upgrading", func() {
			gomock.InOrder(
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil),
			)
			result, err := upgrader.ExternalDependencyAvailabilityCheck(context.TODO(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

})

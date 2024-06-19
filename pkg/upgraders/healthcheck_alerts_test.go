package upgraders

import (
	"fmt"

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
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("HealthCheck Alerts", func() {
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
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseNew).GetUpgradeConfig()
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
		config.HealthCheck = healthCheck{
			IgnoredCriticals:  []string{"alert1", "alert2"},
			IgnoredNamespaces: []string{"ns1"},
		}
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

	Context("When no critical alerts are firing", func() {
		var alertsResponse *metrics.AlertResponse

		JustBeforeEach(func() {
			alertsResponse = &metrics.AlertResponse{}
		})
		It("Prehealth check should pass", func() {
			gomock.InOrder(
				mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, metrics.MetricsQueryFailed, gomock.Any(), gomock.Any()),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, metrics.CriticalAlertsFiring, gomock.Any(), gomock.Any()),
			)
			result, err := CriticalAlerts(mockMetricsClient, upgrader.config, upgradeConfig, logger)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).Should(BeTrue())
		})
	})

	Context("When there are critical alerts are firing", func() {
		var alertsResponse *metrics.AlertResponse
		JustBeforeEach(func() {
			alertsResponse = &metrics.AlertResponse{
				Data: metrics.AlertData{
					Result: []metrics.AlertResult{
						{Metric: make(map[string]string), Value: nil},
						{Metric: make(map[string]string), Value: nil},
					},
				},
			}
		})
		It("Prehealth check should not pass", func() {
			gomock.InOrder(
				mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := CriticalAlerts(mockMetricsClient, upgrader.config, upgradeConfig, logger)
			Expect(err).Should(HaveOccurred())
			Expect(result).Should(BeFalse())
		})
	})

	Context("When unable to query metrics", func() {
		var alertsResponse *metrics.AlertResponse
		var fakeError = fmt.Errorf("fake cannot query metrics error")
		JustBeforeEach(func() {
			alertsResponse = &metrics.AlertResponse{}
		})
		It("Prehealth check should not pass", func() {
			gomock.InOrder(
				mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, fakeError),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := CriticalAlerts(mockMetricsClient, upgrader.config, upgradeConfig, logger)
			Expect(err).Should(HaveOccurred())
			Expect(result).Should(BeFalse())
		})
	})
})

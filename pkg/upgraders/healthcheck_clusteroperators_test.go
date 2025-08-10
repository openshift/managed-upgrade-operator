package upgraders

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	gomock "go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"

	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("HealthCheck ClusterOperators", func() {
	var (
		logger            logr.Logger
		mockCtrl          *gomock.Controller
		mockMetricsClient *mockMetrics.MockMetrics
		mockCVClient      *cvMocks.MockClusterVersion

		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig

		// upgrader to be used during tests
		config  *upgraderConfig
		version string
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseNew).GetUpgradeConfig()
		mockCtrl = gomock.NewController(GinkgoT())
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		config = buildTestUpgraderConfig(90, 30, 8, 120, 30)
		config.HealthCheck = healthCheck{
			IgnoredCriticals:  []string{"alert1", "alert2"},
			IgnoredNamespaces: []string{"ns1"},
		}
		version = "mockVersion"
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When no ClusterOperators are degraded", func() {
		It("Prehealth check will pass", func() {
			gomock.InOrder(
				mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, metrics.ClusterOperatorsStatusFailed, gomock.Any(), gomock.Any()),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, metrics.ClusterOperatorsDegraded, gomock.Any(), gomock.Any()),
			)
			result, err := ClusterOperators(mockMetricsClient, mockCVClient, upgradeConfig, logger, version)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).Should(BeTrue())
		})
	})

	Context("When there are ClusterOperators degraded", func() {
		It("Prehealth check will fail", func() {
			gomock.InOrder(
				mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{"test-clusteroperator"}}, nil),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := ClusterOperators(mockMetricsClient, mockCVClient, upgradeConfig, logger, version)
			Expect(err).Should(HaveOccurred())
			Expect(result).Should(BeFalse())
		})
	})

	Context("When unable to fetch status of clusteroperators ", func() {
		var fakeError = fmt.Errorf("fake cannot fetch clusteroperators error")
		It("Prehealth check will fail", func() {
			gomock.InOrder(
				mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, fakeError),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := ClusterOperators(mockMetricsClient, mockCVClient, upgradeConfig, logger, version)
			Expect(err).Should(HaveOccurred())
			Expect(result).Should(BeFalse())
		})
	})
})

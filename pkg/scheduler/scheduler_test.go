package scheduler

import (
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
)

var _ = Describe("Scheduler", func() {
	var (
		mockCtrl      *gomock.Controller
		metricsClient *mockMetrics.MockMetrics
		upgradeConfig *upgradev1alpha1.UpgradeConfig
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		metricsClient = mockMetrics.NewMockMetrics(mockCtrl)
	})

	It("should be ready to upgrade if upgradeAt is 10 mins before now", func() {
		metricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()).AnyTimes()
		metricsClient.EXPECT().UpdateMetricUpgradeWindowBreached(gomock.Any()).AnyTimes()
		s := &scheduler{}
		upgradeConfig = testUpgradeConfig(true, time.Now().Add(-10*time.Minute).Format(time.RFC3339))
		result := s.IsReadyToUpgrade(upgradeConfig, metricsClient, 60*time.Minute)
		Expect(result).To(BeTrue())
	})
	It("should be not ready to upgrade if upgradeAt is 80 mins before now", func() {
		metricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()).AnyTimes()
		metricsClient.EXPECT().UpdateMetricUpgradeWindowBreached(gomock.Any()).AnyTimes()
		s := &scheduler{}
		upgradeConfig = testUpgradeConfig(true, time.Now().Add(80*time.Minute).Format(time.RFC3339))
		result := s.IsReadyToUpgrade(upgradeConfig, metricsClient, 60*time.Minute)
		Expect(result).To(BeFalse())
	})
	It("it should not be ready to upgrade if proceed is set to false", func() {
		metricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()).AnyTimes()
		metricsClient.EXPECT().UpdateMetricUpgradeWindowBreached(gomock.Any()).AnyTimes()
		s := &scheduler{}
		upgradeConfig = testUpgradeConfig(false, time.Now().Format(time.RFC3339))
		result := s.IsReadyToUpgrade(upgradeConfig, metricsClient, 60*time.Minute)
		Expect(result).To(BeFalse())
	})
})

func testUpgradeConfig(proceed bool, upgradeAt string) *upgradev1alpha1.UpgradeConfig {
	return &upgradev1alpha1.UpgradeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgradeconfig-example",
		},
		Spec: upgradev1alpha1.UpgradeConfigSpec{
			Proceed:   proceed,
			UpgradeAt: upgradeAt,
		},
	}
}

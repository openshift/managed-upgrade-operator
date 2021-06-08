package scheduler

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
)

var _ = Describe("Scheduler", func() {
	var (
		upgradeConfig *upgradev1alpha1.UpgradeConfig
	)

	It("should be ready to upgrade if upgradeAt is 10 mins before now", func() {
		s := &scheduler{}
		upgradeConfig = testUpgradeConfig(true, time.Now().Add(-10*time.Minute).Format(time.RFC3339))
		result := s.IsReadyToUpgrade(upgradeConfig, 60*time.Minute)
		Expect(result.IsReady).To(BeTrue())
	})
	It("should be not ready to upgrade if upgradeAt is 80 mins before now", func() {
		s := &scheduler{}
		upgradeConfig = testUpgradeConfig(true, time.Now().Add(80*time.Minute).Format(time.RFC3339))
		result := s.IsReadyToUpgrade(upgradeConfig, 60*time.Minute)
		Expect(result.IsReady).To(BeFalse())
	})
	It("it should not be ready to upgrade and indicate breach if upgradeAt is after timeout", func() {
		s := &scheduler{}
		upgradeConfig = testUpgradeConfig(true, time.Now().Add(-10*time.Minute).Format(time.RFC3339))
		result := s.IsReadyToUpgrade(upgradeConfig, 5*time.Minute)
		Expect(result.IsReady).To(BeTrue())
		Expect(result.IsBreached).To(BeTrue())
	})
})

func testUpgradeConfig(proceed bool, upgradeAt string) *upgradev1alpha1.UpgradeConfig {
	return &upgradev1alpha1.UpgradeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgradeconfig-example",
		},
		Spec: upgradev1alpha1.UpgradeConfigSpec{
			UpgradeAt: upgradeAt,
		},
	}
}

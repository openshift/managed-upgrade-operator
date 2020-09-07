package upgradeconfig

import (
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeConfigController", func() {

	var (
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
	})

	Context("Update", func() {
		Context("When the old object meta doesn't exist", func() {
			It("will not return true", func() {
				result := StatusChangedPredicate.UpdateFunc(event.UpdateEvent{MetaOld: nil, ObjectOld: upgradeConfig, MetaNew: upgradeConfig.GetObjectMeta(), ObjectNew: upgradeConfig})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the old object doesn't exist", func() {
			It("will not return true", func() {
				result := StatusChangedPredicate.UpdateFunc(event.UpdateEvent{MetaOld: upgradeConfig.GetObjectMeta(), ObjectOld: nil, MetaNew: upgradeConfig.GetObjectMeta(), ObjectNew: upgradeConfig})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the new object meta doesn't exist", func() {
			It("will not return true", func() {
				result := StatusChangedPredicate.UpdateFunc(event.UpdateEvent{MetaOld: upgradeConfig.GetObjectMeta(), ObjectOld: upgradeConfig, MetaNew: nil, ObjectNew: upgradeConfig})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the new object doesn't exist", func() {
			It("will not return true", func() {
				result := StatusChangedPredicate.UpdateFunc(event.UpdateEvent{MetaOld: upgradeConfig.GetObjectMeta(), ObjectOld: upgradeConfig, MetaNew: upgradeConfig.GetObjectMeta(), ObjectNew: nil})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the old and new events match", func() {
			It("will return true", func() {
				uc1 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				uc2 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				result := StatusChangedPredicate.UpdateFunc(event.UpdateEvent{MetaOld: uc1.GetObjectMeta(), ObjectOld: uc1, MetaNew: uc2.GetObjectMeta(), ObjectNew: uc2})
				Expect(result).To(BeTrue())
			})
		})
		Context("When the old and new events do not match", func() {
			It("will not return true", func() {
				uc1 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				uc2 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				uc2.Status.History = []upgradev1alpha1.UpgradeHistory{{Version: "something else"}}
				result := StatusChangedPredicate.UpdateFunc(event.UpdateEvent{MetaOld: uc1.GetObjectMeta(), ObjectOld: uc1, MetaNew: uc2.GetObjectMeta(), ObjectNew: uc2})
				Expect(result).To(BeFalse())
			})
		})

	})

})

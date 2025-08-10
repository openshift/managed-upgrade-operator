package nodekeeper

import (
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NodeKeeperController IgnoreMasterPredicate", func() {

	var (
		masterNode    *corev1.Node
		notMasterNode *corev1.Node
	)

	BeforeEach(func() {
		masterNode = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					machinery.MasterLabel: "",
				},
			},
		}
		notMasterNode = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{},
			},
		}
	})

	Context("IgnoreMasterPredicate", func() {
		It("Update ignores master nodes", func() {
			result := IgnoreMasterPredicate().Update(event.UpdateEvent{ObjectNew: masterNode})
			Expect(result).To(BeFalse())
		})
		It("Create ignores master nodes", func() {
			result := IgnoreMasterPredicate().Create(event.CreateEvent{Object: masterNode})
			Expect(result).To(BeFalse())
		})
		It("Delete ignores master nodes", func() {
			result := IgnoreMasterPredicate().Delete(event.DeleteEvent{Object: masterNode})
			Expect(result).To(BeFalse())
		})
		It("Generic ignores master nodes", func() {
			result := IgnoreMasterPredicate().Generic(event.GenericEvent{Object: masterNode})
			Expect(result).To(BeFalse())
		})

		It("Update allows non master nodes", func() {
			result := IgnoreMasterPredicate().Update(event.UpdateEvent{ObjectNew: notMasterNode})
			Expect(result).To(BeTrue())
		})
		It("Create allows non master nodes", func() {
			result := IgnoreMasterPredicate().Create(event.CreateEvent{Object: notMasterNode})
			Expect(result).To(BeTrue())
		})
		It("Delete allows non master nodes", func() {
			result := IgnoreMasterPredicate().Delete(event.DeleteEvent{Object: notMasterNode})
			Expect(result).To(BeTrue())
		})
		It("Generic allows non master nodes", func() {
			result := IgnoreMasterPredicate().Generic(event.GenericEvent{Object: notMasterNode})
			Expect(result).To(BeTrue())
		})
	})
})

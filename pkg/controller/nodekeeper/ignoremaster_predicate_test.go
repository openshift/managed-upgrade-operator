package nodekeeper

import (
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/ginkgo"
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
			result := IgnoreMasterPredicate.UpdateFunc(event.UpdateEvent{MetaNew: masterNode})
			Expect(result).To(BeFalse())
		})
		It("Create ignores master nodes", func() {
			result := IgnoreMasterPredicate.CreateFunc(event.CreateEvent{Meta: masterNode.GetObjectMeta()})
			Expect(result).To(BeFalse())
		})
		It("Delete ignores master nodes", func() {
			result := IgnoreMasterPredicate.DeleteFunc(event.DeleteEvent{Meta: masterNode.GetObjectMeta()})
			Expect(result).To(BeFalse())
		})
		It("Generic ignores master nodes", func() {
			result := IgnoreMasterPredicate.GenericFunc(event.GenericEvent{Meta: masterNode.GetObjectMeta()})
			Expect(result).To(BeFalse())
		})

		It("Update allows non master nodes", func() {
			result := IgnoreMasterPredicate.UpdateFunc(event.UpdateEvent{MetaNew: notMasterNode})
			Expect(result).To(BeTrue())
		})
		It("Create allows non master nodes", func() {
			result := IgnoreMasterPredicate.CreateFunc(event.CreateEvent{Meta: notMasterNode.GetObjectMeta()})
			Expect(result).To(BeTrue())
		})
		It("Delete allows non master nodes", func() {
			result := IgnoreMasterPredicate.DeleteFunc(event.DeleteEvent{Meta: notMasterNode.GetObjectMeta()})
			Expect(result).To(BeTrue())
		})
		It("Generic allows non master nodes", func() {
			result := IgnoreMasterPredicate.GenericFunc(event.GenericEvent{Meta: notMasterNode.GetObjectMeta()})
			Expect(result).To(BeTrue())
		})
	})
})

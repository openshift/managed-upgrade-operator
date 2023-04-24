package drain

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pod Predicates", func() {

	var (
		pod corev1.Pod
	)

	Context("When testing pod predicates", func() {
		BeforeEach(func() {
			pod = corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
				Spec: corev1.PodSpec{},
			}
		})
		Context("testing if pod namespace is allowed", func() {
			It("allows pods with namespaces not in the ignore list", func() {
				r := containsIgnoredNamespace(pod, []string{"not-same-as-pod", "also-not-the-same"})
				Expect(r).To(BeTrue())
			})
			It("allows pods if there are no namespaces being ignored", func() {
				r := containsIgnoredNamespace(pod, []string{})
				Expect(r).To(BeTrue())
			})
			It("ignore pods with namespaces in the ignore list", func() {
				r := containsIgnoredNamespace(pod, []string{"not-same-as-pod", "test-namespace"})
				Expect(r).To(BeFalse())
			})
			It("ignore pods if the namespace matches a regular expression", func() {
				r := containsIgnoredNamespace(pod, []string{"test-n.+"})
				Expect(r).To(BeFalse())
			})
		})
	})
})

package drain

import (
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
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
		Context("testing containsMatchLabel", func() {
			It("does not panic when a PDB has a nil selector", func() {
				pdbList := &policyv1.PodDisruptionBudgetList{
					Items: []policyv1.PodDisruptionBudget{
						{
							Spec: policyv1.PodDisruptionBudgetSpec{
								Selector: nil,
							},
						},
					},
				}
				Expect(func() {
					containsMatchLabel(pod, pdbList)
				}).ShouldNot(Panic())
				r := containsMatchLabel(pod, pdbList)
				Expect(r).To(BeFalse())
			})
			It("returns true when pod labels match a PDB selector", func() {
				pod.Labels = map[string]string{"app": "test"}
				pdbList := &policyv1.PodDisruptionBudgetList{
					Items: []policyv1.PodDisruptionBudget{
						{
							Spec: policyv1.PodDisruptionBudgetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "test"},
								},
							},
						},
					},
				}
				r := containsMatchLabel(pod, pdbList)
				Expect(r).To(BeTrue())
			})
			It("returns false when pod labels do not match any PDB selector", func() {
				pod.Labels = map[string]string{"app": "other"}
				pdbList := &policyv1.PodDisruptionBudgetList{
					Items: []policyv1.PodDisruptionBudget{
						{
							Spec: policyv1.PodDisruptionBudgetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": "test"},
								},
							},
						},
					},
				}
				r := containsMatchLabel(pod, pdbList)
				Expect(r).To(BeFalse())
			})
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

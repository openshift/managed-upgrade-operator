package upgradeconfig

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("UpgradeConfigController", func() {
	var (
		reconciler reconcile.Reconciler
		testScheme *runtime.Scheme
	)

	BeforeEach(func() {
		testScheme = runtime.NewScheme()
		_ = upgradev1alpha1.SchemeBuilder.AddToScheme(testScheme)
	})
	Context("Reconcile", func() {
		var upgradeConfigName types.NamespacedName

		BeforeEach(func() {
			upgradeConfigName = types.NamespacedName{
				Name:      "test-upgradeconfig",
				Namespace: "test-namespace",
			}

			reconciler, _ = NewReconcileUpgradeConfig(
				fake.NewFakeClientWithScheme(testScheme, &upgradev1alpha1.UpgradeConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      upgradeConfigName.Name,
						Namespace: upgradeConfigName.Namespace,
					},
				}),
				testScheme,
			)
		})
		It("Returns without error", func() {
			_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

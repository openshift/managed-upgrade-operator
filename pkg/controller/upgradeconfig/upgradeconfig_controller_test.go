package upgradeconfig

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
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
		if err := configv1.Install(testScheme); err != nil {
			log.Error(err, "")
			os.Exit(1)
		}

		if err := routev1.Install(testScheme); err != nil {
			log.Error(err, "")
			os.Exit(1)
		}

		if err := machineapi.AddToScheme(testScheme); err != nil {
			log.Error(err, "")
			os.Exit(1)
		}
		if err := machineconfigapi.Install(testScheme); err != nil {
			log.Error(err, "")
			os.Exit(1)
		}
		_ = upgradev1alpha1.SchemeBuilder.AddToScheme(testScheme)
	})
	Context("Reconcile", func() {
		var upgradeConfigName types.NamespacedName

		BeforeEach(func() {
			upgradeConfigName = types.NamespacedName{
				Name:      "test-upgradeconfig",
				Namespace: "test-namespace",
			}
			//TODO we need add init objects to fake client that the upgrade controller will used
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

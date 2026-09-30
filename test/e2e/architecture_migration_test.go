//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/specprovider"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var _ = ginkgo.Describe("architecture migration", ginkgo.Serial, ginkgo.Label("architecture-migration"), func() {
	ginkgo.It("creates migration history and requests Multi through the controller", ginkgo.SpecTimeout(3*time.Hour), func(ctx context.Context) {
		if os.Getenv("RUN_ARCHITECTURE_MIGRATION_E2E") != "true" {
			ginkgo.Skip("Set RUN_ARCHITECTURE_MIGRATION_E2E=true to migrate a disposable cluster to Multi")
		}

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(configv1.AddToScheme(scheme)).To(Succeed())
		Expect(upgradev1alpha1.AddToScheme(scheme)).To(Succeed())
		restConfig, err := config.GetConfig()
		Expect(err).NotTo(HaveOccurred())
		cluster, err := client.New(restConfig, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())

		ginkgo.By("requiring the local policy provider and an idle single-architecture cluster")
		operatorConfig := &corev1.ConfigMap{}
		Expect(cluster.Get(ctx, client.ObjectKey{Namespace: operatorNamespace, Name: "managed-upgrade-operator-config"}, operatorConfig)).To(Succeed())
		provider := &specprovider.SpecProviderConfig{}
		Expect(yaml.Unmarshal([]byte(operatorConfig.Data["config.yaml"]), provider)).To(Succeed())
		Expect(strings.ToUpper(provider.ConfigManager.Source)).To(Equal(string(specprovider.LOCAL)), "Configure the LOCAL provider before running this test")

		key := client.ObjectKey{Namespace: operatorNamespace, Name: upgradeConfigResourceName}
		err = cluster.Get(ctx, key, &upgradev1alpha1.UpgradeConfig{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "A managed UpgradeConfig must not already exist: %v", err)

		cvKey := client.ObjectKey{Name: "version"}
		cv := &configv1.ClusterVersion{}
		Expect(cluster.Get(ctx, cvKey, cv)).To(Succeed())
		Expect(cv.Status.Desired.Architecture).NotTo(Equal(configv1.ClusterVersionArchitectureMulti), "Use a single-architecture test cluster")
		Expect(cv.Status.Desired.Version).NotTo(BeEmpty())
		Expect(cv.Status.Desired.Image).NotTo(BeEmpty())
		Expect(cv.Status.History).NotTo(BeEmpty())
		Expect(cv.Status.History[0].State).To(Equal(configv1.CompletedUpdate))
		Expect(cv.Status.History[0].Image).To(Equal(cv.Status.Desired.Image))
		if update := cv.Spec.DesiredUpdate; update != nil {
			Expect(update.Architecture).NotTo(Equal(configv1.ClusterVersionArchitectureMulti), "A migration is already requested")
			if update.Version != "" {
				Expect(update.Version).To(Equal(cv.Status.Desired.Version), "A version upgrade is already requested")
			}
			if update.Image != "" {
				Expect(update.Image).To(Equal(cv.Status.Desired.Image), "An image upgrade is already requested")
			}
		}
		version, channel := cv.Status.Desired.Version, cv.Spec.Channel

		ginkgo.By("creating an architecture-only migration request at the current version")
		uc := makeUpgradeConfig(upgradeConfigResourceName, operatorNamespace, time.Now().UTC().Format(time.RFC3339), version, "")
		uc.Spec.Desired.Architecture = configv1.ClusterVersionArchitectureMulti
		Expect(cluster.Create(ctx, &uc)).To(Succeed())
		ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
			current := &upgradev1alpha1.UpgradeConfig{}
			err := cluster.Get(cleanupCtx, key, current)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			if current.UID != uc.UID {
				return
			}
			history := current.Status.History.GetHistoryForUpdate(uc.Spec.Desired)
			if history == nil || history.Phase != upgradev1alpha1.UpgradePhaseUpgraded {
				ginkgo.GinkgoWriter.Printf("Retaining UpgradeConfig %s for ongoing migration or failure diagnosis\n", key)
				return
			}
			Expect(client.IgnoreNotFound(cluster.Delete(cleanupCtx, current))).To(Succeed())
		}, ginkgo.NodeTimeout(2*time.Minute))

		ginkgo.By("verifying that reconciliation creates a distinct architecture history entry")
		Eventually(func(g Gomega) {
			current := &upgradev1alpha1.UpgradeConfig{}
			g.Expect(cluster.Get(ctx, key, current)).To(Succeed())
			g.Expect(current.UID).To(Equal(uc.UID))
			history := current.Status.History.GetHistoryForUpdate(uc.Spec.Desired)
			g.Expect(history).NotTo(BeNil())
			g.Expect(history.Architecture).To(Equal(configv1.ClusterVersionArchitectureMulti))
			g.Expect(history.Version).To(Equal(version))
			g.Expect(history.PrecedingVersion).To(Equal(version))
			g.Expect(history.Phase).To(BeElementOf(upgradev1alpha1.UpgradePhaseNew, upgradev1alpha1.UpgradePhasePending, upgradev1alpha1.UpgradePhaseUpgrading, upgradev1alpha1.UpgradePhaseUpgraded))
		}).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		ginkgo.By("verifying the controller submits the migration request to CVO")
		Eventually(func(g Gomega) {
			g.Expect(cluster.Get(ctx, cvKey, cv)).To(Succeed())
			g.Expect(cv.Spec.DesiredUpdate).NotTo(BeNil())
			g.Expect(cv.Spec.DesiredUpdate.Architecture).To(Equal(configv1.ClusterVersionArchitectureMulti))
			g.Expect(cv.Spec.DesiredUpdate.Version).To(Equal(version))
			g.Expect(cv.Spec.DesiredUpdate.Image).To(BeEmpty())
			g.Expect(cv.Spec.DesiredUpdate.Force).To(BeFalse())
			g.Expect(cv.Spec.Channel).To(Equal(channel))
		}).WithContext(ctx).WithTimeout(20 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		ginkgo.By("waiting for CVO and MUO to report the migration complete")
		Eventually(func(g Gomega) {
			g.Expect(cluster.Get(ctx, cvKey, cv)).To(Succeed())
			g.Expect(cv.Status.Desired.Architecture).To(Equal(configv1.ClusterVersionArchitectureMulti))
			g.Expect(cv.Status.Desired.Version).To(Equal(version))
			g.Expect(cv.Status.History).NotTo(BeEmpty())
			g.Expect(cv.Status.History[0].State).To(Equal(configv1.CompletedUpdate))
			g.Expect(cv.Status.History[0].Image).To(Equal(cv.Status.Desired.Image))
			current := &upgradev1alpha1.UpgradeConfig{}
			g.Expect(cluster.Get(ctx, key, current)).To(Succeed())
			history := current.Status.History.GetHistoryForUpdate(uc.Spec.Desired)
			g.Expect(history).NotTo(BeNil())
			g.Expect(history.Phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgraded))
			g.Expect(history.CompleteTime).NotTo(BeNil())
		}).WithContext(ctx).WithTimeout(2 * time.Hour).WithPolling(15 * time.Second).Should(Succeed())
	})
})

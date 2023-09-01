// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/openshift/api/config/v1"
	config "github.com/openshift/client-go/config/clientset/versioned"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"github.com/openshift/osde2e-common/pkg/clients/prometheus"
	"github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	operatorName                           = "managed-upgrade-operator"
	operatorNamespace                      = "openshift-managed-upgrade-operator"
	operatorLockFile                       = "managed-upgrade-operator-lock"
	upgradeConfigResourceName              = "managed-upgrade-config"
	upgradeConfigForDedicatedAdminTestName = "osde2e-da-upgrade-config"
	rolePrefix                             = "managed-upgrade-operator"
)

var (
	k8s                        *openshift.Client
	impersonatedResourceClient *openshift.Client
	prom                       *prometheus.Client
	clusterVersion             *apiv1.ClusterVersion
	err                        error
	upgradeConfig              upgradev1alpha1.UpgradeConfig
)

var _ = ginkgo.Describe("managed-upgrade-operator", ginkgo.Ordered, func() {
	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)
		var err error
		k8s, err = openshift.New(ginkgo.GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup k8s client")
		Expect(upgradev1alpha1.AddToScheme(k8s.GetScheme())).Should(Succeed(), "Unable to register upgradev1alpha1 api scheme")
		impersonatedResourceClient, err = k8s.Impersonate("test-user@redhat.com", "dedicated-admins")
		Expect(err).ShouldNot(HaveOccurred(), "Unable to setup impersonated k8s client")
		Expect(upgradev1alpha1.AddToScheme(impersonatedResourceClient.GetScheme())).Should(Succeed(), "Unable to register upgradev1alpha1 api to impersonated client scheme")
		prom, err = prometheus.New(ctx, k8s)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup prometheus client")
	})

	ginkgo.It("is installed", func(ctx context.Context) {
		ginkgo.By("Checking the namespace exists")
		err := k8s.Get(ctx, operatorNamespace, operatorNamespace, &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", operatorNamespace)

		ginkgo.By("Checking the operator lock file config map exists")
		assertions.EventuallyConfigMap(ctx, k8s, operatorLockFile, operatorNamespace).WithTimeout(300*time.Second).WithPolling(30*time.Second).Should(Not(BeNil()), "configmap %s should exist", operatorLockFile)

		ginkgo.By("Checking the operator deployment exists and is available")
		assertions.EventuallyDeployment(ctx, k8s, operatorName, operatorNamespace)

		// this operator's clusterroles have a version suffix, so only check the prefix
		ginkgo.By("Checking the role exists")
		var roles rbacv1.RoleList
		err = k8s.WithNamespace(operatorNamespace).List(ctx, &roles)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(&roles).Should(ContainItemWithPrefix(rolePrefix), "Roles with prefix %s should exist", rolePrefix)

		ginkgo.By("Checking the rolebinding exists")
		var rolebindings rbacv1.RoleBindingList
		err = k8s.List(ctx, &rolebindings)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(&rolebindings).Should(ContainItemWithPrefix(rolePrefix), "Rolebindings with prefix %s should exist", rolePrefix)
	})

	ginkgo.When("dedicated admin attempts,", func() {
		ginkgo.It("should not let them modify upgradeconfig", func(ctx context.Context) {
			// Add the upgradeconfig to the cluster
			upgradeConfig := makeMinimalUpgradeConfig(upgradeConfigForDedicatedAdminTestName, operatorNamespace)
			ginkgo.By("Trying to create upgrade config")
			err = impersonatedResourceClient.Create(ctx, &upgradeConfig)
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "Expected forbidden error: dedicated admin should not be able to create upgradeconfig")

			ginkgo.By("Adding test upgrade config as cluster admin")
			err = k8s.Create(ctx, &upgradeConfig)
			Expect(err).NotTo(HaveOccurred(), "Upgradeconfig should be created")

			ginkgo.By("Trying to delete upgrade config")
			err = impersonatedResourceClient.Delete(ctx, &upgradeConfig)
			Expect(apierrors.IsForbidden(err)).To(BeTrue(), "Expected forbidden error: dedicated admin should not be able to delete upgradeconfig")

			ginkgo.By("Cleaning up upgrade config")
			err = k8s.Delete(ctx, &upgradeConfig)
			Expect(err).NotTo(HaveOccurred(), "Could not clean up upgradeconfig")
		})
	})

	ginkgo.When("upgrade config is received,", func() {
		ginkgo.BeforeEach(func(ctx context.Context) {
			ginkgo.By("Retrieving clusterversion")
			cfg, err := config.NewForConfig(k8s.GetConfig())
			Expect(err).NotTo(HaveOccurred(), "Could not create k8s clientset")
			clusterVersion, err = cfg.ConfigV1().ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
			// Validate clusterversion
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterVersion).NotTo(BeNil())
			skipIfNoUpdatesOrCurrentlyUpgrading(ctx, clusterVersion)
		})

		ginkgo.It("should stay Pending if the start time is in future", func(ctx context.Context) {
			targetVersion := clusterVersion.Status.AvailableUpdates[0].Version
			targetChannel := clusterVersion.Spec.Channel
			startTime := time.Now().UTC().Add(12 * time.Hour)
			upgradeConfig = makeUpgradeConfig(upgradeConfigResourceName, operatorNamespace, startTime.Format(time.RFC3339), targetVersion, targetChannel)

			ginkgo.By("Creating test upgrade config")
			err = k8s.Create(ctx, &upgradeConfig)
			Expect(err).NotTo(HaveOccurred(), "Upgradeconfig should be created")

			ginkgo.By("Polling upgrade config")
			var pollUc upgradev1alpha1.UpgradeConfig
			Eventually(k8s.Get(ctx, upgradeConfigResourceName, operatorNamespace, &pollUc)).WithTimeout(60 * time.Second).WithPolling(3 * time.Second).Should(BeNil())
			Eventually(func() *upgradev1alpha1.UpgradeHistory {
				k8s.Get(ctx, upgradeConfigResourceName, operatorNamespace, &pollUc)
				return pollUc.Status.History.GetHistory(targetVersion)
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).ShouldNot(BeNil())
			Eventually(func() upgradev1alpha1.UpgradePhase {
				k8s.Get(ctx, upgradeConfigResourceName, operatorNamespace, &pollUc)
				return pollUc.Status.History.GetHistory(targetVersion).Phase
			}).WithTimeout(60 * time.Second).WithPolling(5 * time.Second).Should(Equal((upgradev1alpha1.UpgradePhasePending)))
		})

		ginkgo.It("should raise prometheus metric if start time is invalid", func(ctx context.Context) {
			targetVersion := clusterVersion.Status.AvailableUpdates[0].Version
			targetChannel := clusterVersion.Spec.Channel
			upgradeConfig = makeUpgradeConfig(upgradeConfigResourceName, operatorNamespace, "this is not a start time", targetVersion, targetChannel)

			ginkgo.By("Creating test upgrade config")
			err = k8s.WithNamespace(operatorNamespace).Create(ctx, &upgradeConfig)
			Expect(err).NotTo(HaveOccurred(), "Upgradeconfig should be created")

			ginkgo.By("Polling prometheus query")
			query := fmt.Sprintf("upgradeoperator_upgradeconfig_validation_failed{upgradeconfig_name=\"%s\"} == 1", upgradeConfigResourceName)
			Eventually(func(ctx context.Context) bool {
				context, cancel := context.WithTimeout(ctx, 1*time.Minute)
				defer cancel()
				vector, err := prom.InstantQuery(context, query)
				return err == nil && vector.Len() == 1
			}).WithContext(ctx).WithTimeout(60*time.Second).WithPolling(5*time.Second).Should(BeTrue(),
				"MUO should raise prometheus metric for invalid start time for upgrade config", upgradeConfigResourceName)
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			err := k8s.Get(ctx, upgradeConfigResourceName, operatorNamespace, &upgradeConfig)
			if err == nil {
				ginkgo.By("Cleaning up upgrade config")
				err := k8s.Delete(ctx, &upgradeConfig)
				Expect(err).NotTo(HaveOccurred(), "Could not clean up upgrade config")
			}
		})
	})

	ginkgo.It("can be upgraded", func(ctx context.Context) {
		err := k8s.UpgradeOperator(ctx, operatorName, operatorNamespace)
		Expect(err).NotTo(HaveOccurred(), "operator upgrade failed")
	})

})

// Make upgrade config with given target
func makeUpgradeConfig(name string, ns string, startTime string, version string, channel string) upgradev1alpha1.UpgradeConfig {
	uc := upgradev1alpha1.UpgradeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: upgradev1alpha1.UpgradeConfigSpec{
			Desired: upgradev1alpha1.Update{
				Version: version,
				Channel: channel,
			},
			UpgradeAt:            startTime,
			PDBForceDrainTimeout: 60,
			Type:                 upgradev1alpha1.OSD,
		},
	}
	return uc
}

// Make generic minimal upgradeconfig
func makeMinimalUpgradeConfig(name string, ns string) upgradev1alpha1.UpgradeConfig {
	uc := upgradev1alpha1.UpgradeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: upgradev1alpha1.UpgradeConfigSpec{
			Type: upgradev1alpha1.OSD,
		},
	}
	return uc
}

// Skip spec if (a) the cluster has no updates available (b) there is an existing upgrade config
func skipIfNoUpdatesOrCurrentlyUpgrading(ctx context.Context, clusterVersion *apiv1.ClusterVersion) {
	if len(clusterVersion.Status.AvailableUpdates) == 0 {
		ginkgo.Skip("Skipping, cluster has no available updates")
	}
	err := k8s.Get(ctx, upgradeConfigResourceName, operatorNamespace, &upgradeConfig)
	if err != nil {
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Unexpected error getting upgrade config")
	} else {
		ginkgo.Skip("Skipping due to existing UpgradeConfig. Cluster seems to be post-upgrade.")
	}
}

package upgraders

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("PostUpgradeStep", func() {

	var (
		testUpgrader       *clusterUpgrader
		testUpgraderConfig *upgraderConfig
		log                logr.Logger
		testFileIntegrity  *unstructured.Unstructured
		configClient       *fake.ClientBuilder
		fioClient          *fake.ClientBuilder
	)

	BeforeEach(func() {
		log = logf.Log.WithName("upgrader-test-logger")

		testFileIntegrity = &unstructured.Unstructured{}
		testFileIntegrity.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "fileintegrity.openshift.io",
			Kind:    "FileIntegrity",
			Version: "v1alpha1",
		})
		testFileIntegrity.Object = map[string]interface{}{
			"apiVersion": "fileintegrity.openshift.io/v1alpha1",
			"kind":       "FileIntegrity",
			"metadata": map[string]interface{}{
				"name":      fioObject,
				"namespace": fioNamespace,
			},
		}

		configClient = fake.NewClientBuilder()
		fioClient = fake.NewClientBuilder().WithRuntimeObjects(testFileIntegrity)

	})

	Context("When the managed-upgrade-operator-config is configured with fedramp as true", func() {
		It("FIO should be re-initialized", func() {
			testUpgraderConfig = &upgraderConfig{Environment: environment{Fedramp: true}}
			testUpgrader = &clusterUpgrader{client: configClient.Build(), config: testUpgraderConfig}

			isFr := testUpgrader.config.Environment.IsFedramp()
			Expect(isFr).To(BeTrue())

			testUpgrader.client = fioClient.Build()
			err := testUpgrader.postUpgradeFIOReInit(context.TODO(), log)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When the managed-upgrade-operator-config is configured with fedramp as false", func() {
		It("FIO should not be re-initialized", func() {
			testUpgraderConfig = &upgraderConfig{Environment: environment{Fedramp: false}}
			testUpgrader = &clusterUpgrader{client: configClient.Build(), config: testUpgraderConfig}

			isFr := testUpgrader.config.Environment.IsFedramp()
			Expect(isFr).To(BeFalse())

			testUpgrader.client = fioClient.Build()
			err := testUpgrader.postUpgradeFIOReInit(context.TODO(), log)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

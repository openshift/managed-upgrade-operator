package upgraders

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/managed-upgrade-operator/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("PostUpgradeStep", func() {

	var (
		testConfig         *ugConfigManagerSpec
		testOperatorConfig *corev1.ConfigMap
		testUpgrader       *clusterUpgrader
		log                logr.Logger
		testFileIntegrity  *unstructured.Unstructured
		configClient       *fake.ClientBuilder
		fioClient          *fake.ClientBuilder
	)

	BeforeEach(func() {
		log = logf.Log.WithName("upgrader-test-logger")

		testConfig = &ugConfigManagerSpec{
			Config: ugConfigManager{
				Source:     "OCM",
				OcmBaseURL: "https://api.openshiftusgov.com",
			},
		}

		testOperatorConfig = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.ConfigMapName,
				Namespace: config.OperatorNamespace,
			},
			Data: map[string]string{
				"config.yaml": `
                  configManager:
                    source: ` + testConfig.Config.Source + `
                    ocmBaseUrl: ` + testConfig.Config.OcmBaseURL,
			},
		}
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

		configClient = fake.NewClientBuilder().WithRuntimeObjects(testOperatorConfig)
		fioClient = fake.NewClientBuilder().WithRuntimeObjects(testFileIntegrity)

	})

	Context("When the managed-upgrade-operator-config Source is OCM", func() {
		Context("When the OCM Base URL belongs to FedRAMP", func() {
			It("FIO should be re-initialized", func() {
				Expect(testConfig.Config.Source).To(Equal("OCM"))
				Expect(testConfig.Config.OcmBaseURL).To(Equal("https://api.openshiftusgov.com"))

				testUpgrader = &clusterUpgrader{client: configClient.Build()}

				isFr, err := testUpgrader.frClusterCheck(context.TODO())
				Expect(isFr).To(BeTrue())
				Expect(err).NotTo(HaveOccurred())

				testUpgrader.client = fioClient.Build()
				err = testUpgrader.postUpgradeFIOReInit(context.TODO(), log)
				Expect(err).NotTo(HaveOccurred())

			})
		})
		Context("When the OCM Base URL does not belong to FedRAMP", func() {
			BeforeEach(func() {
				testConfig.Config.OcmBaseURL = "https://api.openshift.com"
				testOperatorConfig.Data["config.yaml"] = `
                  configManager: 
                    source: ` + testConfig.Config.Source + `
                    ocmBaseUrl: ` + testConfig.Config.OcmBaseURL
			})
			It("FIO re-init step should be skipped", func() {
				Expect(testConfig.Config.Source).To(Equal("OCM"))
				Expect(testConfig.Config.OcmBaseURL).NotTo(Equal("https://api.openshiftusgov.com"))

				testUpgrader = &clusterUpgrader{client: configClient.Build()}
				isFr, err := testUpgrader.frClusterCheck(context.TODO())
				Expect(isFr).To(BeFalse())
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("When the managed-upgrade-operator-config Source is not OCM", func() {
		BeforeEach(func() {
			testConfig.Config.Source = "TEST"
			testOperatorConfig.Data["config.yaml"] = `
              configManager: 
                source: ` + testConfig.Config.Source + `
                ocmBaseUrl: ` + testConfig.Config.OcmBaseURL
		})
		It("FIO re-init step should be skipped", func() {
			Expect(testConfig.Config.Source).NotTo(Equal("OCM"))

			testUpgrader = &clusterUpgrader{client: configClient.Build()}
			isFr, err := testUpgrader.frClusterCheck(context.TODO())
			Expect(isFr).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

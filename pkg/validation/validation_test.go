package validation

import (
	"github.com/blang/semver"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Validation of UpgradeConfig CR", func() {

	const (
		testCompletedUpdate configv1.UpdateState = "Completed"
	)

	var (
		testValidator         Validator
		testUpgradeConfig     *upgradev1alpha1.UpgradeConfig
		testUpgradeConfigName types.NamespacedName
		testClusterVersion    *configv1.ClusterVersion
		testLogger            logr.Logger
	)

	BeforeEach(func() {
		testValidator = &validator{}
		testUpgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		testLogger = logf.Log.WithName("Config Validation Test Logger")

		// testUpgradeConfig is used to test various fields for validation
		testUpgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(testUpgradeConfigName).GetUpgradeConfig()

		// testClusterVersion is used to test various fields for validation
		testClusterVersion = &configv1.ClusterVersion{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: testUpgradeConfig.Spec.Desired.Version + "different"},
				Channel:       testUpgradeConfig.Spec.Desired.Channel + "different",
			},
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
				},
				History: []configv1.UpdateHistory{
					{
						State:   testCompletedUpdate,
						Version: testUpgradeConfig.Spec.Desired.Version,
					},
				},
			},
		}
	})

	Context("Validating UpgradeAt timestamp", func() {
		Context("When the UpgradeAt timestamp is NOT RFC3339 format", func() {
			It("Validation is false and error is returned as NOT nil", func() {
				// Set UpgradeAt as non RFC3339 format
				testUpgradeConfig.Spec.UpgradeAt = "sometime tomorrow morning would be great thanks"

				ok, err := testValidator.IsValidUpgradeConfig(testUpgradeConfig, testClusterVersion, testLogger)
				Expect(err).ShouldNot(BeNil())
				Expect(ok).Should(BeFalse())
			})
		})
		Context("Validating UpgradeConfig desired version", func() {
			Context("When getting the current cluster version fails", func() {
				It("Validation is false and error is returned as NOT nil", func() {
					// Set version as empty string
					testClusterVersion.Status.History[0].Version = ""
					ok, err := testValidator.IsValidUpgradeConfig(testUpgradeConfig, testClusterVersion, testLogger)
					Expect(err).ShouldNot(BeNil())
					Expect(ok).Should(BeFalse())
				})
			})
		})
		Context("Validating versions are semver", func() {
			Context("When the UpgradeConfig version is NOT valid", func() {
				It("Validation is false and error is returned as NOT nil", func() {
					// Set version as non semver
					testUpgradeConfig.Spec.Desired.Version = "not a correct semver"
					ok, err := testValidator.IsValidUpgradeConfig(testUpgradeConfig, testClusterVersion, testLogger)
					Expect(err).ShouldNot(BeNil())
					Expect(ok).Should(BeFalse())
				})
			})
			Context("When the ClusterVersion version is NOT valid", func() {
				It("Validation is false and error is returned as NOT nil", func() {
					// Set version as non semver
					testUpgradeConfig.Spec.Desired.Version = "4.4.4"
					testClusterVersion.Status.History[0].Version = "not a correct semver"
					ok, err := testValidator.IsValidUpgradeConfig(testUpgradeConfig, testClusterVersion, testLogger)
					Expect(err).ShouldNot(BeNil())
					Expect(ok).Should(BeFalse())
				})
			})
		})
		Context("Comparing versions", func() {
			Context("When desired version is less then current version", func() {
				It("Returns proceed as false", func() {
					// Set desired < current
					desiredVersion, _ := semver.Parse("4.4.4")
					currentVersion, _ := semver.Parse("4.4.5")
					proceed := compareVersions(desiredVersion, currentVersion, testLogger)
					Expect(proceed).Should(BeFalse())
				})
			})
			Context("When desired version is equal to current version", func() {
				It("Returns proceed as false", func() {
					// Set desired == current
					desiredVersion, _ := semver.Parse("4.4.4")
					currentVersion, _ := semver.Parse("4.4.4")
					proceed := compareVersions(desiredVersion, currentVersion, testLogger)
					Expect(proceed).Should(BeFalse())
				})
			})
			Context("When desired version is greater then current version", func() {
				It("Returns proceed as true", func() {
					// Set desired == current
					desiredVersion, _ := semver.Parse("4.4.5")
					currentVersion, _ := semver.Parse("4.4.4")
					proceed := compareVersions(desiredVersion, currentVersion, testLogger)
					Expect(proceed).Should(BeTrue())
				})
			})

		})
	})
})

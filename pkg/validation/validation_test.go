package validation

import (
	"net/http"
	"time"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		testClient            client.Client
		server                *ghttp.Server
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
						State: testCompletedUpdate,
						StartedTime: v1.Time{
							Time: time.Now().UTC().Add(-60 * time.Minute),
						},
						CompletionTime: &v1.Time{
							Time: time.Now().UTC().Add(-60 * time.Minute),
						},
						Version:  "some bad version",
						Verified: false,
					},
					{
						State: testCompletedUpdate,
						StartedTime: v1.Time{
							Time: time.Now().UTC(),
						},
						CompletionTime: &v1.Time{
							Time: time.Now().UTC(),
						},
						Version:  testUpgradeConfig.Spec.Desired.Version,
						Verified: false,
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

				result, err := testValidator.IsValidUpgradeConfig(testClient, testUpgradeConfig, testClusterVersion, testLogger)
				Expect(err).ShouldNot(BeNil())
				Expect(result.IsValid).Should(BeFalse())
			})
		})
	})
	Context("Validating UpgradeConfig desired version", func() {
		Context("When getting the current cluster version fails", func() {
			It("Validation is false and error is returned", func() {
				// Set version as empty string
				// It shouldn't pick the first element, as it's older
				testClusterVersion.Status.History[1].Version = ""
				result, err := testValidator.IsValidUpgradeConfig(testClient, testUpgradeConfig, testClusterVersion, testLogger)
				Expect(err).ShouldNot(BeNil())
				Expect(result.IsValid).Should(BeFalse())
			})
		})
	})
	Context("Validating versions are semver", func() {
		Context("When the UpgradeConfig version is NOT valid", func() {
			It("Validation is false and error is returned as NOT nil", func() {
				// Set version as non semver
				testUpgradeConfig.Spec.Desired.Version = "not a correct semver"
				result, err := testValidator.IsValidUpgradeConfig(testClient, testUpgradeConfig, testClusterVersion, testLogger)
				Expect(err).ShouldNot(BeNil())
				Expect(result.IsValid).Should(BeFalse())
			})
		})
		Context("When the ClusterVersion version is NOT valid", func() {
			It("Validation is false and error is returned as NOT nil", func() {
				// Set version as non semver
				testUpgradeConfig.Spec.Desired.Version = "4.4.4"
				testClusterVersion.Status.History[0].Version = "not a correct semver"
				result, err := testValidator.IsValidUpgradeConfig(testClient, testUpgradeConfig, testClusterVersion, testLogger)
				Expect(err).ShouldNot(BeNil())
				Expect(result.IsValid).Should(BeFalse())
			})
		})
	})
	Context("Comparing versions", func() {
		Context("When desired version is less then current version", func() {
			It("Indicates a downgrade", func() {
				// Set desired < current
				desiredVersion, _ := semver.Parse("4.4.4")
				currentVersion, _ := semver.Parse("4.4.5")
				versionCompare, err := compareVersions(desiredVersion, currentVersion, testLogger)
				Expect(versionCompare).Should(Equal(VersionDowngrade))
				Expect(err).Should(BeNil())
			})
		})
		Context("When desired version is equal to current version", func() {
			It("Returns proceed as false", func() {
				// Set desired == current
				desiredVersion, _ := semver.Parse("4.4.4")
				currentVersion, _ := semver.Parse("4.4.4")
				versionCompare, err := compareVersions(desiredVersion, currentVersion, testLogger)
				Expect(versionCompare).Should(Equal(VersionEqual))
				Expect(err).Should(BeNil())
			})
		})
		Context("When desired version is greater then current version", func() {
			It("Returns proceed as true", func() {
				// Set desired == current
				desiredVersion, _ := semver.Parse("4.4.5")
				currentVersion, _ := semver.Parse("4.4.4")
				versionCompare, err := compareVersions(desiredVersion, currentVersion, testLogger)
				Expect(versionCompare).Should(Equal(VersionUpgrade))
				Expect(err).Should(BeNil())
			})
		})
	})
	Context("Validating ClusterVersion Upstream configuration", func() {
		Context("When ClusterVersion Upstream is defined explicitly", func() {
			It("Explicit value is returned", func() {
				testClusterVersion.Spec.Upstream = "http://example-server/"
				result := getUpstreamURL(testClusterVersion)
				Expect(result).Should(Equal(string(testClusterVersion.Spec.Upstream)))
			})
		})
		Context("When the ClusterVersion Upstream is empty", func() {
			It("Default value is returned", func() {
				result := getUpstreamURL(testClusterVersion)
				Expect(result).Should(Equal(defaultUpstreamServer))
			})
		})
	})

	Context("Validating image", func() {
		Context("when the image format is valid", func() {
			It("Should pass the validation", func() {
				testImage := "example.com/test-ns/test-image@sha256:aaaabbbbccccddddeeeeffff111122223333444455556666aaaabbbbccccdddd"
				err := imageValidation(testImage)
				Expect(err).ShouldNot(HaveOccurred())
			})
		})
		Context("when the image format is incorrect", func() {
			It("Should fail validate if the image format is incorrect", func() {
				testImage := "www.example.com/test"
				err := imageValidation(testImage)
				Expect(err).Should(HaveOccurred())
			})
		})
	})

	Context("Validating channel", func() {
		Context("When the specified version is found in CVO", func() {
			It("should pass the channel validation", func() {
				cvoUpdates := []configv1.Update{
					{
						Version: "4.7.19",
						Image:   "test.registry/test-namespace/test-image@sha256:eafdac268e1f65053de423ba4a028e8de5133ab78e7954d76ed838bcf5f4f666",
					},
					{
						Version: "4.7.16",
						Image:   "test.registry/test-namespace/test-image@sha256:3e59cff6101b0f0732540d9f2cf1fe9c7ea5ab1e8737df82e789eeb129d1a9af",
					},
				}
				testUpgradeConfig.Spec.Desired.Version = "4.7.19"
				err := channelValidation(testUpgradeConfig, cvoUpdates, testLogger)
				Expect(err).ShouldNot(HaveOccurred())
			})
		})
		Context("When the specified version is not found in CVO", func() {
			It("should fail the channel validation", func() {
				cvoUpdates := []configv1.Update{
					{
						Version: "4.7.19",
						Image:   "test.registry/test-namespace/test-image@sha256:eafdac268e1f65053de423ba4a028e8de5133ab78e7954d76ed838bcf5f4f666",
					},
					{
						Version: "4.7.16",
						Image:   "test.registry/test-namespace/test-image@sha256:3e59cff6101b0f0732540d9f2cf1fe9c7ea5ab1e8737df82e789eeb129d1a9af",
					},
				}
				testUpgradeConfig.Spec.Desired.Version = "4.7.18"
				err := channelValidation(testUpgradeConfig, cvoUpdates, testLogger)
				Expect(err).Should(HaveOccurred())
			})
		})
	})
	Context("Sending the http request to remote server", func() {
		BeforeEach(func() {
			server = ghttp.NewServer()
		})
		AfterEach(func() {
			server.Close()
		})
		Context("When the server url is available", func() {
			BeforeEach(func() {
				statuscode := http.StatusOK
				body := []byte("response body")
				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/path"),
					ghttp.RespondWithPtr(&statuscode, &body),
				))
			})
			It("Will return the expected result", func() {
				result, err := runHTTP("http://" + server.Addr() + "/path")
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result).Should(ContainSubstring("body"))
			})
		})
		Context("When the return code is non 200", func() {
			BeforeEach(func() {
				statuscode := http.StatusInternalServerError
				body := []byte("body string")
				server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/path"),
					ghttp.RespondWithPtr(&statuscode, &body),
				))
			})
			It("Will report error", func() {
				_, err := runHTTP("http://" + server.Addr() + "/path")
				Expect(err).ShouldNot(BeNil())
			})
		})
	})

})

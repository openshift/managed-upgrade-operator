package notifier

import (
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OCM Config", func() {
	Describe("OcmNotifierConfig", func() {
		var (
			config OcmNotifierConfig
		)

		BeforeEach(func() {
			config = OcmNotifierConfig{
				ConfigManager: OcmNotifierConfigManager{
					OcmBaseUrl: "https://example.com",
				},
			}
		})

		Context("IsValid", func() {
			It("should return nil error for a valid config", func() {
				err := config.IsValid()
				Expect(err).To(BeNil())
			})
		})

		Context("GetOCMBaseURL", func() {
			It("should return the OCM base URL", func() {
				expectedURL, _ := url.Parse("https://example.com")
				result := config.GetOCMBaseURL()
				Expect(result).To(Equal(expectedURL))
			})
		})

		Context("IsValid error", func() {
			It("should return error for a valid config", func() {
				configerr := OcmNotifierConfig{
					ConfigManager: OcmNotifierConfigManager{
						OcmBaseUrl: "  http://example.com/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z",
					},
				}
				err := configerr.IsValid()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("OcmFeatureConfig", func() {
		var (
			config OcmFeatureConfig
		)

		BeforeEach(func() {
			config = OcmFeatureConfig{
				OCMFeatureGate: OcmFeatureGates{
					Enabled: []string{"feature1", "feature2"},
				},
			}
		})

		Context("IsValid", func() {
			It("should return nil error for a valid config", func() {
				err := config.IsValid()
				Expect(err).To(BeNil())
			})
		})
	})
})

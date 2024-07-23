package ocmprovider

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const baseUrl = "http://test.ocp"

var _ = Describe("Config", func() {
	var (
		mockCtrl    *gomock.Controller
		ocmProvider OcmProviderConfig
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		// Initialing temp value for config's baseurl
		ocmProvider = OcmProviderConfig{
			ConfigManager: ConfigManager{
				OcmBaseUrl: baseUrl,
			},
		}

	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("GetOCMBaseURL", func() {
		// Validating IsValid func
		It("Testing the IsValid function", func() {
			err := ocmProvider.IsValid()
			Expect(err).To(BeNil())
		})
		// Validating GetOCMBaseURL func
		It("Get base url", func() {
			url := ocmProvider.GetOCMBaseURL()
			Expect(url).To(Not(BeNil()))
		})

		// Validating IsValid func
		It("Testing the IsValid function error", func() {
			ocmProviderErr := OcmProviderConfig{
				ConfigManager: ConfigManager{
					OcmBaseUrl: " http://example.com/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z",
				},
			}
			err := ocmProviderErr.IsValid()
			Expect(err).To(HaveOccurred())
		})
	})

})

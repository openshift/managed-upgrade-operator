package notifier

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const baseUrl = "http://test.ocp"

var _ = Describe("Config", func() {
	var (
		mockCtrl          *gomock.Controller
		ocmNotifierConfig OcmNotifierConfig
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		// Initialing temp value for config's baseurl
		ocmNotifierConfig = OcmNotifierConfig{
			ConfigManager: OcmNotifierConfigManager{
				OcmBaseUrl: baseUrl,
			},
		}

	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("GetOCMBaseURL", func() {
		// Validating IsValid func - success
		It("Testing the IsValid function", func() {
			err := ocmNotifierConfig.IsValid()
			Expect(err).To(BeNil())
		})
		// Validating GetOCMBaseURL func
		It("Get base url", func() {
			url := ocmNotifierConfig.GetOCMBaseURL()
			Expect(url).To(Not(BeNil()))
		})

		// Validating IsValid func - failure
		It("Testing the IsValid function error", func() {
			ocmNotifierConfigErr := OcmNotifierConfig{
				ConfigManager: OcmNotifierConfigManager{
					OcmBaseUrl: " http://example.com/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z",
				},
			}
			err := ocmNotifierConfigErr.IsValid()
			Expect(err).To(HaveOccurred())
		})
	})

})

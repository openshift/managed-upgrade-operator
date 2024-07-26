package ocm

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const baseUrl string = "https://mockapi.openshift.com"

var _ = Describe("Config", func() {

	var (
		mockCtrl *gomock.Controller
		config   OcmClientConfig
	)

	BeforeEach(func() {

		mockCtrl = gomock.NewController(GinkgoT())

		config = OcmClientConfig{
			ConfigManager: ConfigManager{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Get OCM Base URL", func() {
		It("Test isvalid function -  success scenario", func() {
			config.ConfigManager.OcmBaseUrl = baseUrl
			err := config.IsValid()
			Expect(err).To(BeNil())

		})

		It("Test isvalid function - failure scenario", func() {
			config.ConfigManager.OcmBaseUrl = "http[]://fakeapi.openshift.com"
			err := config.IsValid()
			Expect(err).To(HaveOccurred())
		})

		It("Test GetOCMBaseURL function", func() {
			config.ConfigManager.OcmBaseUrl = baseUrl
			url := config.GetOCMBaseURL()
			Expect(url.String()).To(BeEquivalentTo(baseUrl))
		})
	})
})

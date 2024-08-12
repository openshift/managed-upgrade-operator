package notifier

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const (
	ExpectOcm   = "OCM"
	ExpectLocal = "LOCAL"
)

var _ = Describe("Config", func() {
	var (
		mockCtrl       *gomock.Controller
		notifierConfig NotifierConfig
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		// Initialing temp value for Notifier's Config
		notifierConfig = NotifierConfig{
			ConfigManager: NotifierConfigManager{},
		}

	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Notifier config", func() {
		// Return Source value as NIL
		It("Source value is nil", func() {
			notifierConfig.ConfigManager.Source = ""
			err := notifierConfig.IsValid()
			Expect(err).To(BeNil())
		})

		// Return Source value as OCM
		It("Source value = ocm", func() {
			notifierConfig.ConfigManager.Source = "OCM"
			err := notifierConfig.IsValid()
			Expect(err).To(BeNil())
			Expect(notifierConfig.ConfigManager.Source).To(Equal(ExpectOcm))
		})

		// Return Source value as LOCAL
		It("Source value = LOCAL", func() {
			notifierConfig.ConfigManager.Source = "LOCAL"
			err := notifierConfig.IsValid()
			Expect(err).To(BeNil())
			Expect(notifierConfig.ConfigManager.Source).To(Equal(ExpectLocal))
		})

		// Error with configuration
		It("No valid configured notifier", func() {
			notifierConfig.ConfigManager.Source = "ERROR"
			err := notifierConfig.IsValid()
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(ErrNoNotifierConfigured))
		})
	})

})

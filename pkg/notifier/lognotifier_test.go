package notifier

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const (
	value       = MuoStateStarted
	description = "Testing"
)

var _ = Describe("logNotifier", func() {
	var (
		mockCtrl *gomock.Controller
		lNotify  logNotifier
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("LogNotifier", func() {
		// Test NewLogNotifier returns a new logNotifier
		It("NewLogNotifier return struct", func() {
			logNotifier, err := NewLogNotifier()
			Expect(err).To(BeNil())
			Expect(logNotifier).To(Equal(&lNotify))
		})

		// Testing NotifyState func
		It("Testing NotifyState func", func() {
			err := lNotify.NotifyState(value, description)
			Expect(err).To(BeNil())
		})
	})
})

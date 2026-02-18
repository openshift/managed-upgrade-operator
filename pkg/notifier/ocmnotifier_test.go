package notifier

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OCM Notifier", func() {

	Context("State mapping", func() {
		It("maps OCM state to MUO state", func() {
			ms, ok := mapState(OcmStateStarted, stateMap)
			Expect(ok).To(BeTrue())
			Expect(ms).To(Equal(MuoStateStarted))
		})

		It("returns false for invalid OCM state", func() {
			_, ok := mapState("invalid", stateMap)
			Expect(ok).To(BeFalse())
		})
	})

	Context("State validation", func() {
		It("allows transition from scheduled to started", func() {
			result := validateStateTransition(MuoStateScheduled, MuoStateStarted)
			Expect(result).To(BeTrue())
		})

		It("blocks invalid transition from pending", func() {
			result := validateStateTransition(MuoStatePending, MuoStateCompleted)
			Expect(result).To(BeFalse())
		})

		It("blocks transition from completed state", func() {
			result := validateStateTransition(MuoStateCompleted, MuoStateStarted)
			Expect(result).To(BeFalse())
		})
	})

	Context("Service Log State mapping", func() {
		It("maps MUO state to ServiceLog state correctly", func() {
			slState, ok := mapSLState(MuoStateControlPlaneUpgradeStartedSL, serviceLogMap)
			Expect(ok).To(BeTrue())
			Expect(slState).To(Equal(ServiceLogStateControlPlaneStarted))
		})

		It("returns false for invalid state", func() {
			_, ok := mapSLState("InvalidState", serviceLogMap)
			Expect(ok).To(BeFalse())
		})
	})

	Context("toString function", func() {
		It("converts MuoState to string", func() {
			result := toString(MuoStateStarted)
			Expect(result).To(Equal("StateStarted"))
		})
	})
})

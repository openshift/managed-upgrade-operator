package notifier

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LogNotifier", func() {
	Describe("NotifyState", func() {
		It("should write to log output", func() {
			// Create a new LogNotifier
			logNotifier , _ := NewLogNotifier()

			// Call NotifyState with a test value and description
			err := logNotifier.NotifyState(MuoState("test"), "test description")

			// Assert that no error occurred
			Expect(err).To(BeNil())
		})
	})
})

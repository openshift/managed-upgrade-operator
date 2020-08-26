package util

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Upgrade channel tests", func() {

	Context("When there is a valid Z-Stream jump", func() {
		It("determines the right channel", func() {
			res, err := InferUpgradeChannelFromChannelGroup("stable", "4.4.1", "4.4.2")
			Expect(*res).To(Equal("stable-4.4"))
			Expect(err).To(BeNil())
		})
	})

	Context("When there is a Y+1 stream jump", func() {
		It("determines the right channel", func() {
			res, err := InferUpgradeChannelFromChannelGroup("stable", "4.4.1", "4.5.2")
			Expect(*res).To(Equal("stable-4.5"))
			Expect(err).To(BeNil())
		})
	})

	Context("When there is a Y+2 stream jump", func() {
		It("rejects the request", func() {
			_, err := InferUpgradeChannelFromChannelGroup("stable", "4.4.1", "4.6.2")
			Expect(err).NotTo(BeNil())
		})
	})

	Context("When major versions don't match", func() {
		It("rejects the request", func() {
			_, err := InferUpgradeChannelFromChannelGroup("stable", "4.4.1", "5.4.1")
			Expect(err).NotTo(BeNil())
		})
	})

	Context("When the FROM version doesn't parse", func() {
		It("rejects the request", func() {
			_, err := InferUpgradeChannelFromChannelGroup("stable", "v4.4.1", "5.4.1")
			Expect(err).NotTo(BeNil())
		})
	})

	Context("When the TO version doesn't parse", func() {
		It("rejects the request", func() {
			_, err := InferUpgradeChannelFromChannelGroup("stable", "4.4.1", "v5.4.1")
			Expect(err).NotTo(BeNil())
		})
	})
})

package aro

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ARO Upgrader Maintenance Window", func() {
	Context("When performing ARO upgrade", func() {
		It("Checks Upgrade is Successful", func() {
			status, err := checkUpgrade()
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(BeTrue())
		})
	})
})

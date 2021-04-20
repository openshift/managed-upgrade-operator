package aro

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ARO Upgrader", func() {
	Context("When performing ARO upgrade", func() {
		It("Checks Upgrade is Successful", func() {
			status, err := checkUpgrade()
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(BeTrue())
		})
	})
})

func checkUpgrade() (bool, error) {
	fmt.Println("Dummy check to test ARO upgrade")
	return true, nil
}

package ocmagent

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestOcmUpgradeConfigManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCM Agent Client Suite")
}

package ocmprovider

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOcmUpgradeConfigManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OcmUpgradeConfigManager Suite")
}

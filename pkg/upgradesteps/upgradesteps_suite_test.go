package upgradesteps

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpgraderStepRunner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UpgradeStep Runner Suite")
}

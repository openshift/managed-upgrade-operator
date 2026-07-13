package upgradeconfigmanager

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpgradeConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UpgradeConfigManager Suite")
}

package nodekeeper

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUpgradeConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodeKeeperController Suite")
}

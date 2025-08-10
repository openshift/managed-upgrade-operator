package nodekeeper

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNodeKeeperController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodeKeeperController Suite")
}

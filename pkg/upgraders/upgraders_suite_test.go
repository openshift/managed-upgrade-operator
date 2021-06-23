package upgraders

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestUpgraders(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgraders Suite")
}

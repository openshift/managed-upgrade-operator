package aro

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAro(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ARO Upgrader Suite")
}

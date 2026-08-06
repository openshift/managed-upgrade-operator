package dvo

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDvo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DVO Suite")
}

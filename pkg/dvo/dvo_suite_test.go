package dvo

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDvo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DVO Client Suite")
}

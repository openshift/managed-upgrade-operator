package localprovider

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLocalprovider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Localprovider Suite")
}

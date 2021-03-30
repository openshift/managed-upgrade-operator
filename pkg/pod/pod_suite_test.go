package pod

import (
"testing"

. "github.com/onsi/ginkgo"
. "github.com/onsi/gomega"
)

func TestPod(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pod Suite")
}

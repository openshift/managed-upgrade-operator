//go:build tools
// +build tools

// Place any runtime dependencies as imports in this file.
// Go modules will be forced to download and install them.
package tools

import (
	_ "github.com/onsi/ginkgo/v2"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)

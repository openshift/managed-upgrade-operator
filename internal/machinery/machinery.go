// Package machinery provides upgrade related functions that are abstracted from machineconfig.
package machinery

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/machinery.go -package=mocks github.com/openshift/managed-upgrade-operator/internal/machinery Machinery
type Machinery interface {
	IsUpgrading(c client.Client, nodeType string, logger logr.Logger) (bool, error)
}

type machinery struct{}

func NewMachinery() Machinery {
	return &machinery{}
}

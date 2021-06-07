// Package machinery provides upgrade related functions that are abstracted from machineconfig.
package machinery

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MasterLabel for master node
	MasterLabel = "node-role.kubernetes.io/master"
)

// Machinery enables an implementation of a Machinery interface
//go:generate mockgen -destination=mocks/machinery.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/machinery Machinery
type Machinery interface {
	IsUpgrading(c client.Client, nodeType string) (*UpgradingResult, error)
	IsNodeCordoned(node *corev1.Node) *IsCordonedResult
}

type machinery struct{}

// NewMachinery returns a machinery struct
func NewMachinery() Machinery {
	return &machinery{}
}

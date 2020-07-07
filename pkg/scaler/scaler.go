package scaler

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/scaler.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/mscaler Scaler
type Scaler interface {
	EnsureScaleUpNodes(client.Client, logr.Logger) (bool, error)
	EnsureScaleDownNodes(client.Client, logr.Logger) (bool, error)
}

func NewScaler() Scaler {
	return &machineSetScaler{}
}

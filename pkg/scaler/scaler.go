package scaler

import (
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/scaler.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/scaler Scaler
type Scaler interface {
	EnsureScaleUpNodes(client.Client, time.Duration, logr.Logger) (bool, error)
	EnsureScaleDownNodes(client.Client, logr.Logger) (bool, error)
}

func NewScaler() Scaler {
	return &machineSetScaler{}
}

type scaleTimeOutError struct {
	message string
}

func (stoErr *scaleTimeOutError) Error() string {
	return stoErr.message
}

func IsScaleTimeOutError(err error) bool {
	_, ok := err.(*scaleTimeOutError)
	return ok
}
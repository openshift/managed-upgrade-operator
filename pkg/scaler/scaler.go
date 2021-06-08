package scaler

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Scaler is an interface that enables implementations of a Scaler
//go:generate mockgen -destination=mocks/scaler.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/scaler Scaler
type Scaler interface {
	EnsureScaleUpNodes(client.Client, time.Duration, logr.Logger) (bool, error)
	EnsureScaleDownNodes(client.Client, drain.NodeDrainStrategy, logr.Logger) (bool, error)
}

// NewScaler returns a Scaler
func NewScaler() Scaler {
	return &machineSetScaler{}
}

type scaleTimeOutError struct {
	message string
}

func (stoErr *scaleTimeOutError) Error() string {
	return stoErr.message
}

// IsScaleTimeOutError returns a bool if the error arg is that of a scaleTimeOutError
func IsScaleTimeOutError(err error) bool {
	_, ok := err.(*scaleTimeOutError)
	return ok
}

// NewScaleTimeOutError returns a scaleTimeOutError
func NewScaleTimeOutError(msg string) *scaleTimeOutError {
	return &scaleTimeOutError{message: msg}
}

type drainTimeOutError struct {
	nodeName string
}

func (dtoErr *drainTimeOutError) Error() string {
	return dtoErr.nodeName
}

func (dtoErr *drainTimeOutError) GetNodeName() string {
	return dtoErr.nodeName
}

// IsDrainTimeOutError returns a drainTimeOutError and bool that is true if the error arg is
// that of a drainTimeOutError
func IsDrainTimeOutError(err error) (*drainTimeOutError, bool) {
	dte, ok := err.(*drainTimeOutError)
	return dte, ok
}

// NewDrainTimeOutError returns a drainTimeOutError
func NewDrainTimeOutError(nodeName string) *drainTimeOutError {
	return &drainTimeOutError{nodeName: nodeName}
}

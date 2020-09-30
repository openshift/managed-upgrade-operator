package scaler

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/scaler.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/scaler Scaler
type Scaler interface {
	EnsureScaleUpNodes(client.Client, time.Duration, logr.Logger) (bool, error)
	EnsureScaleDownNodes(client.Client, drain.NodeDrainStrategy, logr.Logger) (bool, error)
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

func IsDrainTimeOutError(err error) (*drainTimeOutError, bool) {
	dte, ok := err.(*drainTimeOutError)
	return dte, ok
}

func NewDrainTimeOutError(nodeName string) *drainTimeOutError {
	return &drainTimeOutError{nodeName: nodeName}
}

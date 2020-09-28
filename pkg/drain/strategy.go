package drain

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
)

//go:generate mockgen -destination=mocks/nodeDrainStrategyBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/drain NodeDrainStrategyBuilder
type NodeDrainStrategyBuilder interface {
	NewNodeDrainStrategy(c client.Client, uc *upgradev1alpha1.UpgradeConfig, node *corev1.Node, cfg *NodeDrain) (NodeDrainStrategy, error)
}

//go:generate mockgen -destination=mocks/nodeDrainStrategy.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/drain NodeDrainStrategy
type NodeDrainStrategy interface {
	Execute(*metav1.Time) ([]*DrainStrategyResult, error)
	HasFailed(*metav1.Time) (bool, error)
}

//go:generate mockgen -destination=./drainStrategyMock.go -package=drain github.com/openshift/managed-upgrade-operator/pkg/drain DrainStrategy
type DrainStrategy interface {
	Execute() (*DrainStrategyResult, error)
	HasFailed() (bool, error)
}

//go:generate mockgen -destination=./timedDrainStrategyMock.go -package=drain github.com/openshift/managed-upgrade-operator/pkg/drain TimedDrainStrategy
type TimedDrainStrategy interface {
	GetWaitDuration() time.Duration
	GetName() string
	GetDescription() string
	GetStrategy() DrainStrategy
}

func NewBuilder() NodeDrainStrategyBuilder {
	return &drainStrategyBuilder{}
}

type drainStrategyBuilder struct{}

func (dsb *drainStrategyBuilder) NewNodeDrainStrategy(c client.Client, uc *upgradev1alpha1.UpgradeConfig, node *corev1.Node, cfg *NodeDrain) (NodeDrainStrategy, error) {
	ts, err := getOsdTimedStrategies(c, uc, node, cfg)
	if err != nil {
		return nil, err
	}
	return NewOSDDrainStrategy(c, uc, node, cfg, ts)
}

type DrainStrategyResult struct {
	Message string
	HasExecuted bool
}

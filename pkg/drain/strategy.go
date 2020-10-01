package drain

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

//go:generate mockgen -destination=mocks/nodeDrainStrategyBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/drain NodeDrainStrategyBuilder
type NodeDrainStrategyBuilder interface {
	NewNodeDrainStrategy(c client.Client, uc *upgradev1alpha1.UpgradeConfig, cfg *NodeDrain) (NodeDrainStrategy, error)
}

//go:generate mockgen -destination=mocks/nodeDrainStrategy.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/drain NodeDrainStrategy
type NodeDrainStrategy interface {
	Execute(*corev1.Node) ([]*DrainStrategyResult, error)
	HasFailed(*corev1.Node) (bool, error)
}

//go:generate mockgen -destination=./drainStrategyMock.go -package=drain github.com/openshift/managed-upgrade-operator/pkg/drain DrainStrategy
type DrainStrategy interface {
	Execute(*corev1.Node) (*DrainStrategyResult, error)
	IsValid(*corev1.Node) (bool, error)
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

func newTimedStrategy(name string, description string, waitDuration time.Duration, strategy DrainStrategy) TimedDrainStrategy {
	return &timedStrategy{
		name:         name,
		description:  description,
		waitDuration: waitDuration,
		strategy:     strategy,
	}
}

func (dsb *drainStrategyBuilder) NewNodeDrainStrategy(c client.Client, uc *upgradev1alpha1.UpgradeConfig, cfg *NodeDrain) (NodeDrainStrategy, error) {
	pdbList := &policyv1beta1.PodDisruptionBudgetList{}
	err := c.List(context.TODO(), pdbList)
	if err != nil {
		return nil, err
	}

	allPods := &corev1.PodList{}
	err = c.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	defaultOsdPodPredicates := []pod.PodPredicate{isNotDaemonSet}
	isNotPdbPod := isNotPdbPod(pdbList)
	isPdbPod := isPdbPod(pdbList)
	defaultDuration := cfg.GetTimeOutDuration()
	pdbDuration := uc.GetPDBDrainTimeoutDuration()
	ts := []TimedDrainStrategy{
		newTimedStrategy(defaultPodDeleteName, "Default pod deletion", defaultDuration, &podDeletionStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isNotPdbPod),
		}),
		newTimedStrategy(defaultPodFinalizerRemovalName, "Default pod finalizer removal", defaultDuration, &removeFinalizersStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isNotPdbPod),
		}),
		newTimedStrategy(pdbPodDeleteName, "PDB pod deletion", pdbDuration, &podDeletionStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isPdbPod),
		}),
		newTimedStrategy(pdbPodFinalizerRemovalName, "PDB Pod finalizer removal", pdbDuration, &removeFinalizersStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isPdbPod),
		}),
	}

	return NewNodeDrainStrategy(c, cfg, ts)
}

type DrainStrategyResult struct {
	Message     string
	HasExecuted bool
}

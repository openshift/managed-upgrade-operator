package drain

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
	"github.com/openshift/managed-upgrade-operator/pkg/pod"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

// NodeDrainStrategyBuilder enables implementation for a NodeDrainStrategyBuilder
//
//go:generate mockgen -destination=mocks/nodeDrainStrategyBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/drain NodeDrainStrategyBuilder
type NodeDrainStrategyBuilder interface {
	NewNodeDrainStrategy(c client.Client, logger logr.Logger, uc *upgradev1alpha1.UpgradeConfig, cfg *NodeDrain) (NodeDrainStrategy, error)
}

// NodeDrainStrategy enables implementation for a NodeDrainStrategy
//
//go:generate mockgen -destination=mocks/nodeDrainStrategy.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/drain NodeDrainStrategy
type NodeDrainStrategy interface {
	Execute(*corev1.Node, logr.Logger) ([]*DrainStrategyResult, error)
	HasFailed(*corev1.Node, logr.Logger) (bool, error)
}

// DrainStrategy enables implementation for a DrainStrategy
//
//go:generate mockgen -destination=./drainStrategyMock.go -package=drain -self_package=github.com/openshift/managed-upgrade-operator/pkg/drain github.com/openshift/managed-upgrade-operator/pkg/drain DrainStrategy
type DrainStrategy interface {
	Execute(*corev1.Node, logr.Logger) (*DrainStrategyResult, error)
	IsValid(*corev1.Node, logr.Logger) (bool, error)
}

// TimedDrainStrategy enables implementation for a TimedDrainStrategy
//
//go:generate mockgen -destination=./timedDrainStrategyMock.go -package=drain -self_package=github.com/openshift/managed-upgrade-operator/pkg/drain github.com/openshift/managed-upgrade-operator/pkg/drain TimedDrainStrategy
type TimedDrainStrategy interface {
	GetWaitDuration() time.Duration
	GetName() string
	GetDescription() string
	GetStrategy() DrainStrategy
}

// NewBuilder returns a drainStrategyBuilder
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

// NewNodeDrainStrategy returns a NodeDrainStrategy
func (dsb *drainStrategyBuilder) NewNodeDrainStrategy(c client.Client, logger logr.Logger, uc *upgradev1alpha1.UpgradeConfig, cfg *NodeDrain) (NodeDrainStrategy, error) {
	pdbList := &policyv1.PodDisruptionBudgetList{}
	err := c.List(context.TODO(), pdbList)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("unable to list PodDisruptionBudgets/v1")
		} else {
			return nil, err
		}
	}
	cmBuilder := configmanager.NewBuilder()
	ucb := upgradeconfigmanager.NewBuilder()
	// Notification Client Build
	notifier, err := notifier.NewBuilder().New(c, cmBuilder, ucb)
	if err != nil {
		return nil, err
	}
	// Metrics Client Build
	metricsClient, err := metrics.NewBuilder().NewClient(c)
	if err != nil {
		return nil, err
	}
	defaultOsdPodPredicates := []pod.PodPredicate{isNotDaemonSet}
	isNotPdbPod := isNotPdbPod(pdbList)
	isPdbPod := isPdbPod(pdbList)
	isAllowedNamespace := isAllowedNamespace(cfg.IgnoredNamespacePatterns)
	defaultDuration := cfg.GetTimeOutDuration()
	pdbDuration := uc.GetPDBDrainTimeoutDuration() + cfg.GetExpectedDrainDuration()
	ts := []TimedDrainStrategy{
		newTimedStrategy(defaultPodDeleteName, "Default pod deletion", defaultDuration, &podDeletionStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isNotPdbPod, isAllowedNamespace),
		}),
		newTimedStrategy(defaultPodFinalizerRemovalName, "Default pod finalizer removal", defaultDuration, &removeFinalizersStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isNotPdbPod, isAllowedNamespace),
		}),
		newTimedStrategy(stuckTerminatingPodName, "Pod stuck terminating removal", defaultDuration, &stuckTerminatingStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isNotPdbPod, isAllowedNamespace),
		}),
		newTimedStrategy(pdbPodDeleteName, "PDB pod deletion", pdbDuration, &podDeletionStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isPdbPod, isAllowedNamespace),
		}),
		newTimedStrategy(pdbPodFinalizerRemovalName, "PDB Pod finalizer removal", pdbDuration, &removeFinalizersStrategy{
			client:  c,
			filters: append(defaultOsdPodPredicates, isPdbPod, isAllowedNamespace),
		}),
	}

	return NewNodeDrainStrategy(c, cfg, ts, uc, notifier, metricsClient)
}

// DrainStrategyResult holds fields illustrating a drain strategies result
type DrainStrategyResult struct {
	Message     string
	HasExecuted bool
}

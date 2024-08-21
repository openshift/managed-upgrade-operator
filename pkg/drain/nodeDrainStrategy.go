package drain

import (
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/notifier"
)

var (
	defaultPodDeleteName           = "DELETE"
	pdbPodDeleteName               = "PDB-DELETE"
	defaultPodFinalizerRemovalName = "DEFAULT-FINALIZER"
	pdbPodFinalizerRemovalName     = "PDB-FINALIZER"
	stuckTerminatingPodName        = "POD-STUCK-TERMINATING"
)

// NewNodeDrainStrategy returns a new node drain stategy
func NewNodeDrainStrategy(c client.Client, cfg *NodeDrain, ts []TimedDrainStrategy, uc *upgradev1alpha1.UpgradeConfig,
	notifier notifier.Notifier, metricsClient metrics.Metrics) (NodeDrainStrategy, error) {
	return &osdDrainStrategy{
		c,
		machinery.NewMachinery(),
		cfg,
		ts,
		uc,
		notifier,
		metricsClient,
	}, nil
}

type osdDrainStrategy struct {
	client               client.Client
	machinery            machinery.Machinery
	cfg                  *NodeDrain
	timedDrainStrategies []TimedDrainStrategy
	uc                   *upgradev1alpha1.UpgradeConfig
	notifier             notifier.Notifier
	metricsClient        metrics.Metrics
}

func (ds *osdDrainStrategy) Execute(node *corev1.Node, logger logr.Logger) ([]*DrainStrategyResult, error) {
	result := ds.machinery.IsNodeCordoned(node)
	me := &multierror.Error{}
	res := []*DrainStrategyResult{}
	if result.IsCordoned {
		// A cordoned node with an unknown drain time should be treated as an error
		if result.AddedAt == nil {
			return nil, fmt.Errorf("cannot determine drain commencement time for node %v", node.Name)
		}
		me := &multierror.Error{}
		for _, tds := range ds.timedDrainStrategies {
			dsName := tds.GetName()
			expectedTime := result.AddedAt.Add(tds.GetWaitDuration())
			drainStrategyMsg := fmt.Sprintf("drain strategy %v for node %v, commencing drain at %v, execution expected after %v", dsName, node.Name, result.AddedAt, expectedTime)
			if isAfter(result.AddedAt, tds.GetWaitDuration()) {
				logger.Info(fmt.Sprintf("Executing %s", drainStrategyMsg))
				r, err := tds.GetStrategy().Execute(node, logger)
				if err != nil {
					return nil, err
				}
				me = multierror.Append(err, me)
				if r.HasExecuted {
					if dsName == pdbPodDeleteName {
						// Check if a notification for it has been sent successfully - if so, nothing to do
						isNotified, err := ds.metricsClient.IsMetricNotificationEventSentSet(ds.uc.Name,
							string(notifier.MuoStateDelayed), ds.uc.Spec.Desired.Version)
						if err != nil {
							logger.Error(err, "Failed to send the service log about upgrade delay due to node drain grace period")
							return nil, fmt.Errorf("can't check cluster metric NotificationSent: %v", err)
						}
						if !isNotified {
							logger.Info("Sending upgrade delay message about node drain grace period")
							msg := "Node drain grace period might be impacting cluster upgrade. " +
								"Please refer to the article for further details https://access.redhat.com/solutions/7075425"
							err = ds.notifier.NotifyState(notifier.MuoStateDelayed, msg)
							if err != nil {
								logger.Error(err, "Failed to send the service log about upgrade delay due to node drain grace period")
								return nil, err
							}
							// set the notrification send metric
							ds.metricsClient.UpdateMetricNotificationEventSent(ds.uc.Name, string(notifier.MuoStateDelayed), ds.uc.Spec.Desired.Version)
						}

					}
					res = append(res, &DrainStrategyResult{Message: fmt.Sprintf("Executed %s . Result: %s", drainStrategyMsg, r.Message)})
				}
			} else {
				logger.Info(fmt.Sprintf("Will not yet execute %s", drainStrategyMsg))
			}
		}
	}

	return res, me.ErrorOrNil()
}

func (ds *osdDrainStrategy) HasFailed(node *corev1.Node, logger logr.Logger) (bool, error) {
	result := ds.machinery.IsNodeCordoned(node)
	if result.AddedAt == nil {
		return false, nil
	}

	if len(ds.timedDrainStrategies) == 0 {
		return isAfter(result.AddedAt, ds.cfg.GetTimeOutDuration()), nil
	}

	sortedStrategies := sortDuration(ds.timedDrainStrategies)
	var executedStrategies []TimedDrainStrategy
	currentStrategyIndex := 0
	for _, s := range sortedStrategies {
		if isAfter(result.AddedAt, s.GetWaitDuration()) {
			executedStrategies = append(executedStrategies, s)
			currentStrategyIndex++
		}
	}

	pendingStrategies := sortedStrategies[currentStrategyIndex:]
	var validPendingStrategies []TimedDrainStrategy
	for _, tds := range pendingStrategies {
		isValid, err := tds.GetStrategy().IsValid(node, logger)
		if err != nil {
			return false, err
		}
		if isValid {
			validPendingStrategies = append(validPendingStrategies, tds)
		}
	}

	if len(validPendingStrategies) > 0 {
		return false, nil
	}

	if len(executedStrategies) > 0 {
		lastExecutedStrategy := executedStrategies[len(executedStrategies)-1]
		if lastExecutedStrategy.GetWaitDuration()+ds.cfg.GetExpectedDrainDuration() > ds.cfg.GetTimeOutDuration() {
			return isAfter(result.AddedAt, lastExecutedStrategy.GetWaitDuration()+ds.cfg.GetExpectedDrainDuration()), nil
		}
	}

	return isAfter(result.AddedAt, ds.cfg.GetTimeOutDuration()), nil
}

type timedStrategy struct {
	name         string
	description  string
	waitDuration time.Duration
	strategy     DrainStrategy
}

func (ts *timedStrategy) GetWaitDuration() time.Duration {
	return ts.waitDuration
}

func (ts *timedStrategy) GetName() string {
	return ts.name
}

func (ts *timedStrategy) GetDescription() string {
	return ts.description
}

func (ts *timedStrategy) GetStrategy() DrainStrategy {
	return ts.strategy
}

func isAfter(t *metav1.Time, d time.Duration) bool {
	return t != nil && t.Add(d).Before(metav1.Now().Time)
}

func sortDuration(ts []TimedDrainStrategy) []TimedDrainStrategy {
	sortedSlice := []TimedDrainStrategy{}
	sortedSlice = append(sortedSlice, ts...)
	sort.Slice(sortedSlice, func(i, j int) bool {
		iWait := ts[i].GetWaitDuration()
		jWait := ts[j].GetWaitDuration()
		return iWait < jWait
	})

	return sortedSlice
}

package drain

import (
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
)

var (
	defaultPodDeleteName           = "DELETE"
	pdbPodDeleteName               = "PDB-DELETE"
	defaultPodFinalizerRemovalName = "DEFAULT-FINALIZER"
	pdbPodFinalizerRemovalName     = "PDB-FINALIZER"
	stuckTerminatingPodName        = "POD-STUCK-TERMINATING"
)

// NewNodeDrainStrategy returns a new node drain stategy
func NewNodeDrainStrategy(c client.Client, cfg *NodeDrain, ts []TimedDrainStrategy) (NodeDrainStrategy, error) {
	return &osdDrainStrategy{
		c,
		machinery.NewMachinery(),
		cfg,
		ts,
	}, nil
}

type osdDrainStrategy struct {
	client               client.Client
	machinery            machinery.Machinery
	cfg                  *NodeDrain
	timedDrainStrategies []TimedDrainStrategy
}

func (ds *osdDrainStrategy) Execute(node *corev1.Node) ([]*DrainStrategyResult, error) {
	result := ds.machinery.IsNodeCordoned(node)
	me := &multierror.Error{}
	res := []*DrainStrategyResult{}
	if result.IsCordoned {
		me := &multierror.Error{}
		for _, ds := range ds.timedDrainStrategies {
			if isAfter(result.AddedAt, ds.GetWaitDuration()) {
				r, err := ds.GetStrategy().Execute(node)
				me = multierror.Append(err, me)
				if r.HasExecuted {
					res = append(res, &DrainStrategyResult{Message: fmt.Sprintf("Drain strategy %s has been executed. %s", ds.GetDescription(), r.Message)})
				}
			}
		}
	}

	return res, me.ErrorOrNil()
}

func (ds *osdDrainStrategy) HasFailed(node *corev1.Node) (bool, error) {
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
		isValid, err := tds.GetStrategy().IsValid(node)
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

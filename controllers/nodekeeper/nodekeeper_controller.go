package nodekeeper

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/managed-upgrade-operator/config"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName("controller_nodekeeper")

// blank assignment to verify that ReconcileNodeKeeper implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNodeKeeper{}

// ReconcileNodeKeeper reconciles a NodeKeeper object
type ReconcileNodeKeeper struct {
	Client                      client.Client
	ConfigManagerBuilder        configmanager.ConfigManagerBuilder
	Machinery                   machinery.Machinery
	MetricsClientBuilder        metrics.MetricsBuilder
	DrainstrategyBuilder        drain.NodeDrainStrategyBuilder
	UpgradeConfigManagerBuilder upgradeconfigmanager.UpgradeConfigManagerBuilder
	Scheme                      *runtime.Scheme
}

// Reconcile Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeKeeper) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	upgradeConfigManagerClient, err := r.UpgradeConfigManagerBuilder.NewManager(r.Client)
	if err != nil {
		return reconcile.Result{}, err
	}
	uc, err := upgradeConfigManagerClient.Get()
	if err != nil {
		if err == upgradeconfigmanager.ErrUpgradeConfigNotFound {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	upgradeResult, err := r.Machinery.IsUpgrading(r.Client, "worker")
	if err != nil {
		return reconcile.Result{}, err
	}

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	if !(history != nil && history.Phase == upgradev1alpha1.UpgradePhaseUpgrading && upgradeResult.IsUpgrading) {
		return reconcile.Result{}, nil
	}

	// Fetch the Node instance
	node := &corev1.Node{}
	err = r.Client.Get(context.TODO(), request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	result := r.Machinery.IsNodeCordoned(node)
	metricsClient, err := r.MetricsClientBuilder.NewClient(r.Client)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !result.IsCordoned {
		metricsClient.ResetMetricNodeDrainFailed(node.Name)
		return reconcile.Result{}, nil
	}

	target := config.CMTarget{}
	cmTarget, err := target.NewCMTarget()
	if err != nil {
		return reconcile.Result{}, err
	}

	cfm := r.ConfigManagerBuilder.New(r.Client, cmTarget)
	cfg := &nodeKeeperConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !cfg.NodeDrain.DisableDrainStrategies {
		drainStrategy, err := r.DrainstrategyBuilder.NewNodeDrainStrategy(r.Client, reqLogger, uc, &cfg.NodeDrain)
		if err != nil {
			reqLogger.Error(err, "Error while executing drain.")
			return reconcile.Result{}, err
		}

		res, err := drainStrategy.Execute(node, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
		for _, r := range res {
			reqLogger.Info(r.Message)
		}

		hasFailed, err := drainStrategy.HasFailed(node, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
		if hasFailed {
			// If the node.DeletionTimestamp is set NodeDrainFailed metric needs to be reset
			if node.DeletionTimestamp != nil {
				reqLogger.Info(fmt.Sprintf("DeletionTimestamp set for the node %s. Re-setting NodeDrainFailed metric",
					node.Name))
				metricsClient.ResetMetricNodeDrainFailed(node.Name)
			} else {
				reqLogger.Info(fmt.Sprintf("Node drain timed out %s. Alerting.", node.Name))
				// Set metric only for the node going through upgrade
				if r.Machinery.IsNodeUpgrading(node) {
					metricsClient.UpdateMetricNodeDrainFailed(node.Name)
				}
				return reconcile.Result{RequeueAfter: time.Minute * 1}, nil
			}
		} else {
			metricsClient.ResetMetricNodeDrainFailed(node.Name)
		}
	}

	return reconcile.Result{RequeueAfter: time.Minute * 1}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileNodeKeeper) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(IgnoreMasterPredicate()).
		Complete(r)
}

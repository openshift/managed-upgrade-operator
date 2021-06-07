package nodekeeper

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/managed-upgrade-operator/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

var log = logf.Log.WithName("controller_nodekeeper")

// Add creates a new NodeKeeper Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	kubeConfig := controllerruntime.GetConfigOrDie()
	c, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		return err
	}
	return add(mgr, newReconciler(mgr, c))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, client client.Client) reconcile.Reconciler {
	return &ReconcileNodeKeeper{
		client:                      client,
		configManagerBuilder:        configmanager.NewBuilder(),
		machinery:                   machinery.NewMachinery(),
		metricsClientBuilder:        metrics.NewBuilder(),
		drainstrategyBuilder:        drain.NewBuilder(),
		upgradeConfigManagerBuilder: upgradeconfigmanager.NewBuilder(),
		scheme:                      mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("nodekeeper-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Node, status change will not trigger a reconcile
	err = c.Watch(
		&source.Kind{Type: &corev1.Node{}},
		&handler.EnqueueRequestForObject{},
		IgnoreMasterPredicate)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileNodeKeeper implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileNodeKeeper{}

// ReconcileNodeKeeper reconciles a NodeKeeper object
type ReconcileNodeKeeper struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client                      client.Client
	configManagerBuilder        configmanager.ConfigManagerBuilder
	machinery                   machinery.Machinery
	metricsClientBuilder        metrics.MetricsBuilder
	drainstrategyBuilder        drain.NodeDrainStrategyBuilder
	upgradeConfigManagerBuilder upgradeconfigmanager.UpgradeConfigManagerBuilder
	scheme                      *runtime.Scheme
}

// Reconcile Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeKeeper) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	upgradeConfigManagerClient, err := r.upgradeConfigManagerBuilder.NewManager(r.client)
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

	upgradeResult, err := r.machinery.IsUpgrading(r.client, "worker")
	if err != nil {
		return reconcile.Result{}, err
	}

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	if !(history != nil && history.Phase == upgradev1alpha1.UpgradePhaseUpgrading && upgradeResult.IsUpgrading) {
		return reconcile.Result{}, nil
	}

	// Fetch the Node instance
	node := &corev1.Node{}
	err = r.client.Get(context.TODO(), request.NamespacedName, node)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	result := r.machinery.IsNodeCordoned(node)
	metricsClient, err := r.metricsClientBuilder.NewClient(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !result.IsCordoned {
		metricsClient.ResetMetricNodeDrainFailed(node.Name)
		return reconcile.Result{}, nil
	}

	operatorNamespace, err := util.GetOperatorNamespace()
	if err != nil {
		return reconcile.Result{}, nil
	}
	cfm := r.configManagerBuilder.New(r.client, operatorNamespace)
	cfg := &nodeKeeperConfig{}
	err = cfm.Into(cfg)
	if err != nil {
		return reconcile.Result{}, err
	}

	drainStrategy, err := r.drainstrategyBuilder.NewNodeDrainStrategy(r.client, uc, &cfg.NodeDrain)
	if err != nil {
		reqLogger.Error(err, "Error while executing drain.")
		return reconcile.Result{}, err
	}
	res, err := drainStrategy.Execute(node)
	for _, r := range res {
		reqLogger.Info(r.Message)
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	hasFailed, err := drainStrategy.HasFailed(node)
	if err != nil {
		return reconcile.Result{}, err
	}
	if hasFailed {
		reqLogger.Info(fmt.Sprintf("Node drain timed out %s. Alerting.", node.Name))
		metricsClient.UpdateMetricNodeDrainFailed(node.Name)
		return reconcile.Result{RequeueAfter: time.Minute * 1}, nil
	}

	return reconcile.Result{RequeueAfter: time.Minute * 1}, nil
}

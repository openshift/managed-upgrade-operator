package nodekeeper

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime"
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
		client:               client,
		configManagerBuilder: configmanager.NewBuilder(),
		machinery:            machinery.NewMachinery(),
		metricsClientBuilder: metrics.NewBuilder(),
		drainstrategyBuilder: drain.NewBuilder(),
		scheme:               mgr.GetScheme(),
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
	client               client.Client
	configManagerBuilder configmanager.ConfigManagerBuilder
	machinery            machinery.Machinery
	metricsClientBuilder metrics.MetricsBuilder
	drainstrategyBuilder drain.NodeDrainStrategyBuilder
	scheme               *runtime.Scheme
}

// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeKeeper) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Name", request.Name)

	operatorNamespace, err := getOperatorNamespace()
	if err != nil {
		return reconcile.Result{}, err
	}
	uc, err := getUpgradeConfigCR(r.client, operatorNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
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

// getOperatorNamespace retrieves the operators namespace from an environment
// variable and returns it to the caller.
func getOperatorNamespace() (string, error) {
	envVarOperatorNamespace := "OPERATOR_NAMESPACE"
	ns, found := os.LookupEnv(envVarOperatorNamespace)
	if !found {
		return "", fmt.Errorf("%s must be set", envVarOperatorNamespace)
	}
	return ns, nil
}

func getUpgradeConfigCR(c client.Client, ns string) (*upgradev1alpha1.UpgradeConfig, error) {
	uCList := &upgradev1alpha1.UpgradeConfigList{}

	err := c.List(context.TODO(), uCList, &client.ListOptions{Namespace: ns})
	if err != nil {
		return nil, err
	}

	for _, uC := range uCList.Items {
		return &uC, nil
	}

	return nil, errors.NewNotFound(schema.GroupResource{Group: upgradev1alpha1.SchemeGroupVersion.Group, Resource: "UpgradeConfig"}, "UpgradeConfig")
}

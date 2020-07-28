package nodekeeper

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openshift/managed-upgrade-operator/internal/machinery"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

var log = logf.Log.WithName("controller_nodekeeper")

var (
	pdbForceDrainTimeout int32
)

// Add creates a new NodeKeeper Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileNodeKeeper{
		client:               mgr.GetClient(),
		scheme:               mgr.GetScheme(),
		metricsClientBuilder: metrics.NewBuilder(),
		machinery:            machinery.NewMachinery(),
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
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{}, StatusChangedPredicate)
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
	scheme               *runtime.Scheme
	metricsClientBuilder metrics.MetricsBuilder
	machinery            machinery.Machinery
}

// Reconcile reads that state of the cluster for a UpgradeConfig object and makes changes based on the state read
// and what is in the UpgradeConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileNodeKeeper) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling NodeKeeper")

	// Determine if the cluster is upgrading
	yes, err := r.machinery.IsUpgrading(r.client, "worker", reqLogger)
	if err != nil {
		// An error occurred, return it.
		return reconcile.Result{}, err
	} else if !yes {
		// Nodes are not upgrading.
		return reconcile.Result{}, nil
	}

	reqLogger.Info("Cluster is upgrading. Proceeding.")

	// Initialise metrics
	metricsClient, err := r.metricsClientBuilder.NewClient(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	pdbForceDrainTimeout, err = getPDBForceDrainTimeout(r.client, reqLogger)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("UpgradeConfig not found. No further action.")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	reqLogger.Info("Checking for PodDisruptionBudget alerts.")

	found, err := checkPDBAlerts(metricsClient)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !found {
		log.Info("Found no PDB alerts. No further action")
		return reconcile.Result{}, nil
	}

	log.Info(fmt.Sprintf("Found PodDisruptionBudgetAtLimit Alert, Force Drain at: %d minutes", pdbForceDrainTimeout))

	return reconcile.Result{}, nil

}

func checkPDBAlerts(mC metrics.Metrics) (bool, error) {
	pdbAlertName := "PodDisruptionBudgetAtLimit"
	var pdbAlerts *metrics.AlertResponse
	pdbAlerts, err := mC.Query(fmt.Sprintf("ALERTS{alertname=\"%s\"}", pdbAlertName))

	if err != nil {
		return false, err
	}

	if len(pdbAlerts.Data.Result) == 0 {
		return false, nil
	}

	return true, nil
}

func getUpgradeConfig(c client.Client, logger logr.Logger) (*upgradev1alpha1.UpgradeConfig, error) {
	uCList := &upgradev1alpha1.UpgradeConfigList{}

	err := c.List(context.TODO(), uCList)
	if err != nil {
		return nil, err
	}

	for _, uC := range uCList.Items {
		return &uC, nil
	}

	return nil, errors.NewNotFound(schema.GroupResource{Group: upgradev1alpha1.SchemeGroupVersion.Group, Resource: "UpgradeConfig"}, "UpgradeConfig")
}

func getPDBForceDrainTimeout(c client.Client, logger logr.Logger) (int32, error) {
	uC, err := getUpgradeConfig(c, logger)
	if err != nil {
		return 0, err
	}
	return uC.Spec.PDBForceDrainTimeout, nil

}

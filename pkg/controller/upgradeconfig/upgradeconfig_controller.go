package upgradeconfig

import (
	"context"
	"fmt"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
)

var log = logf.Log.WithName("controller_upgradeconfig")

// Add creates a new UpgradeConfig Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileUpgradeConfig{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

func NewReconcileUpgradeConfig(client client.Client, scheme *runtime.Scheme) (reconcile.Reconciler, error) {
	if scheme == nil {
		return nil, fmt.Errorf("scheme cannot be nil")
	}

	return &ReconcileUpgradeConfig{client, scheme}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("upgradeconfig-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource UpgradeConfig, status change will not trigger a reconcile
	err = c.Watch(&source.Kind{Type: &upgradev1alpha1.UpgradeConfig{}}, &handler.EnqueueRequestForObject{}, StatusChangedPredicate{})
	if err != nil {
		return err
	}
	return nil
}

// blank assignment to verify that ReconcileUpgradeConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileUpgradeConfig{}

// ReconcileUpgradeConfig reconciles a UpgradeConfig object
type ReconcileUpgradeConfig struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a UpgradeConfig object and makes changes based on the state read
// and what is in the UpgradeConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileUpgradeConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling UpgradeConfig")

	upgrader := NewUpgrader()
	// Fetch the UpgradeConfig instance
	instance := &upgradev1alpha1.UpgradeConfig{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// If cluster is already upgrading with different version, we should wait until it completed
	upgrading, err := clusterUpgrading(r.client, instance.Spec.Desired.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	if upgrading {
		return reconcile.Result{}, nil
		reqLogger.Info("cluster is upgrading with different version, cannot upgrade now")
	}

	var history upgradev1alpha1.UpgradeHistory
	found := false
	for _, h := range instance.Status.History {
		if h.Version == instance.Spec.Desired.Version {
			history = h
			found = true
		}
	}
	if !found {
		history = upgradev1alpha1.UpgradeHistory{Version: instance.Spec.Desired.Version, Phase: upgradev1alpha1.UpgradePhaseNew}
		history.Conditions = upgradev1alpha1.NewConditions()
		instance.Status.History = append([]upgradev1alpha1.UpgradeHistory{history}, instance.Status.History...)
		err := r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	status := history.Phase
	reqLogger.Info("current cluster status", "status", status)

	switch status {
	case "", upgradev1alpha1.UpgradePhaseNew, upgradev1alpha1.UpgradePhasePending:
		// TODO verify if it's time to do upgrade, if no, set to "pending", if it's yes, perform upgrade, and set status to "upgrading"
		reqLogger.Info("checking whether it's ready to do upgrade")
		ready := readyToUpgrade(instance)
		if ready {
			reqLogger.Info("it's ready to start upgrade now", "time", time.Now())

			upgrader.UpgradeCluster(r.client, instance, reqLogger)

		} else {
			r.updateStatusPending(reqLogger, instance)
			return reconcile.Result{}, nil
		}
	case upgradev1alpha1.UpgradePhaseUpgrading:
		reqLogger.Info("it's upgrading now")
		upgrader.UpgradeCluster(r.client, instance, reqLogger)
	case upgradev1alpha1.UpgradePhaseUpgraded:
		reqLogger.Info("cluster is already upgraded")
		return reconcile.Result{}, nil
	case upgradev1alpha1.UpgradePhaseFailed:
		reqLogger.Info("the cluster failed the upgrade")
		return reconcile.Result{}, nil
	default:
		reqLogger.Info("unknown status")
	}

	return reconcile.Result{}, nil
}

type StatusChangedPredicate struct {
	predicate.Funcs
}

// Update implements default UpdateEvent filter for validating generation change
func (StatusChangedPredicate) Update(e event.UpdateEvent) bool {
	if e.MetaOld == nil {
		log.Error(nil, "Update event has no old metadata", "event", e)
		return false
	}
	if e.ObjectOld == nil {
		log.Error(nil, "Update event has no old runtime object to update", "event", e)
		return false
	}
	if e.ObjectNew == nil {
		log.Error(nil, "Update event has no new runtime object for update", "event", e)
		return false
	}
	if e.MetaNew == nil {
		log.Error(nil, "Update event has no new metadata", "event", e)
		return false
	}
	newUp := e.ObjectNew.(*upgradev1alpha1.UpgradeConfig)
	oldUp := e.ObjectOld.(*upgradev1alpha1.UpgradeConfig)
	if !reflect.DeepEqual(newUp.Status, oldUp.Status) {
		return false
	}
	return true
}

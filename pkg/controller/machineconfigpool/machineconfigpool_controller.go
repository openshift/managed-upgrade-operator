package machineconfigpool

import (
	"context"
	"time"

	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

var log = logf.Log.WithName("controller_machineconfigpool")

// Add creates a new MachineConfigPool Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileMachineConfigPool{
		client:                      mgr.GetClient(),
		scheme:                      mgr.GetScheme(),
		upgradeConfigManagerBuilder: ucm.NewBuilder(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("machineconfigpool-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource MachineConfigPool
	err = c.Watch(&source.Kind{Type: &machineconfigapi.MachineConfigPool{}}, &handler.EnqueueRequestForObject{}, isWorkerPredicate)
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileMachineConfigPool implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMachineConfigPool{}

// ReconcileMachineConfigPool reconciles a MachineConfigPool object
type ReconcileMachineConfigPool struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client                      client.Client
	scheme                      *runtime.Scheme
	upgradeConfigManagerBuilder ucm.UpgradeConfigManagerBuilder
}

// Reconcile reconciles the machineconfigpool object
func (r *ReconcileMachineConfigPool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MachineConfigPool")

	// Fetch the MachineConfigPool instance
	instance := &machineconfigapi.MachineConfigPool{}
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

	ucManager, err := r.upgradeConfigManagerBuilder.NewManager(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	uc, err := ucManager.Get()
	if err != nil {
		if err == ucm.ErrUpgradeConfigNotFound {
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	if uc.Status.History != nil {
		history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
		if history != nil && history.Phase == upgradev1alpha1.UpgradePhaseUpgrading {
			if instance.Status.UpdatedMachineCount == 0 {
				if history.WorkerStartTime == nil {
					history.WorkerStartTime = &metav1.Time{Time: time.Now()}
				}
			}

			if instance.Status.MachineCount == instance.Status.UpdatedMachineCount {
				if history.WorkerStartTime != nil && history.WorkerCompleteTime == nil {
					history.WorkerCompleteTime = &metav1.Time{Time: time.Now()}
				}
			}
			uc.Status.History.SetHistory(*history)
			err = r.client.Status().Update(context.TODO(), uc)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

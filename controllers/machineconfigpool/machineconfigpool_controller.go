package machineconfigpool

import (
	"context"
	"time"

	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var log = logf.Log.WithName("controller_machineconfigpool")

// blank assignment to verify that ReconcileMachineConfigPool implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileMachineConfigPool{}

// ReconcileMachineConfigPool reconciles a MachineConfigPool object
type ReconcileMachineConfigPool struct {
	Client                      client.Client
	Scheme                      *runtime.Scheme
	UpgradeConfigManagerBuilder ucm.UpgradeConfigManagerBuilder
}

func (r *ReconcileMachineConfigPool) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling MachineConfigPool")

	// Fetch the MachineConfigPool instance
	instance := &machineconfigapi.MachineConfigPool{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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

	ucManager, err := r.UpgradeConfigManagerBuilder.NewManager(r.Client)
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
			err = r.Client.Status().Update(context.TODO(), uc)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileMachineConfigPool) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&machineconfigapi.MachineConfigPool{}).
		WithEventFilter(isWorkerPredicate()).
		Complete(r)
}

package upgradeconfig

import (
	"context"
	"github.com/go-logr/logr"
	"time"

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
	cub "github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader_builder"
)

var log = logf.Log.WithName("controller_upgradeconfig")

// Add creates a new UpgradeConfig Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileUpgradeConfig{
		client:                 mgr.GetClient(),
		scheme:                 mgr.GetScheme(),
		clusterUpgraderBuilder: cub.NewBuilder(),
	}
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
	client                 client.Client
	scheme                 *runtime.Scheme
	clusterUpgraderBuilder cub.ClusterUpgraderBuilder
}

// Reconcile reads that state of the cluster for a UpgradeConfig object and makes changes based on the state read
// and what is in the UpgradeConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileUpgradeConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling UpgradeConfig")

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
	reqLogger.Info("Current upgrade status", "status", status)

	switch status {
	case upgradev1alpha1.UpgradePhaseNew, upgradev1alpha1.UpgradePhasePending:
		// TODO verify if it's time to do upgrade, if no, set to "pending", if it's yes, perform upgrade, and set status to "upgrading"
		reqLogger.Info("Checking whether it's time to do upgrade")
		ready := isReadyToUpgrade(instance)
		if ready {
			now := time.Now()
			reqLogger.Info("It's ready to start upgrade now", "time", now)

			upgrader, err := r.clusterUpgraderBuilder.NewClient(r.client, instance.Spec.Type)
			if err != nil {
				return reconcile.Result{}, err
			}

			history.Phase = upgradev1alpha1.UpgradePhaseUpgrading
			history.StartTime = &metav1.Time{Time: now}
			instance.Status.History.SetHistory(history)
			err = r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}

			return r.upgradeCluster(upgrader, instance, reqLogger)
		} else {
			err := r.updateStatusPending(reqLogger, instance)
			if err != nil {
				// TODO updateStatusPending implements nothing so far so below logged message to be changed when implemented
				reqLogger.Info("Failed to set pending status for upgrade!")
			}
			return reconcile.Result{}, nil
		}
	case upgradev1alpha1.UpgradePhaseUpgrading:
		reqLogger.Info("It's upgrading now")
		upgrader, err := r.clusterUpgraderBuilder.NewClient(r.client, instance.Spec.Type)
		if err != nil {
			return reconcile.Result{}, err
		}

		return r.upgradeCluster(upgrader, instance, reqLogger)
	case upgradev1alpha1.UpgradePhaseUpgraded:
		reqLogger.Info("Cluster is already upgraded")
		return reconcile.Result{}, nil
	default:
		reqLogger.Info("Unknown status")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileUpgradeConfig) upgradeCluster(upgrader cub.ClusterUpgrader, uc *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (reconcile.Result, error) {
	phase, condition, ucErr := upgrader.UpgradeCluster(uc, logger)

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	history.Conditions = upgradev1alpha1.Conditions{*condition}
	history.Phase = phase
	if phase == upgradev1alpha1.UpgradePhaseUpgraded {
		history.CompleteTime = &metav1.Time{Time: time.Now()}
	}
	uc.Status.History.SetHistory(*history)
	err := r.client.Status().Update(context.TODO(), uc)
	if err != nil {
		return reconcile.Result{}, err
	}
	if ucErr != nil {
		return reconcile.Result{}, ucErr
	}

	return reconcile.Result{}, nil
}

func isReadyToUpgrade(upgradeConfig *upgradev1alpha1.UpgradeConfig) bool {
	if !upgradeConfig.Spec.Proceed {
		log.Info("upgrade cannot be proceed", "proceed", upgradeConfig.Spec.Proceed)
		return false
	}
	upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		log.Error(err, "failed to parse spec.upgradeAt", upgradeConfig.Spec.UpgradeAt)
		return false
	}
	now := time.Now()
	if now.After(upgradeTime) && upgradeTime.Add(30*time.Minute).After(now) {
		return true
	}
	return false
}

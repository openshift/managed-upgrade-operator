package upgradeconfig

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
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
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/osd_cluster_upgrader"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
)

var (
	log = logf.Log.WithName("controller_upgradeconfig")
)

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
		metricsClientBuilder:   metrics.NewBuilder(),
		clusterUpgraderBuilder: cub.NewBuilder(),
		validationBuilder:      validation.NewBuilder(),
		configManagerBuilder:   configmanager.NewBuilder(),
		scheduler:              scheduler.NewScheduler(),
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
	metricsClientBuilder   metrics.MetricsBuilder
	clusterUpgraderBuilder cub.ClusterUpgraderBuilder
	validationBuilder      validation.ValidationBuilder
	configManagerBuilder   configmanager.ConfigManagerBuilder
	scheduler              scheduler.Scheduler
}

// Reconcile reads that state of the cluster for a UpgradeConfig object and makes changes based on the state read
// and what is in the UpgradeConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileUpgradeConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling UpgradeConfig")

	// Initialise metrics
	metricsClient, err := r.metricsClientBuilder.NewClient(r.client)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch the UpgradeConfig instance
	instance := &upgradev1alpha1.UpgradeConfig{}
	err = r.client.Get(context.TODO(), request.NamespacedName, instance)
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

	reqLogger.Info("Current cluster status", "status", status)

	switch status {
	case upgradev1alpha1.UpgradePhaseNew, upgradev1alpha1.UpgradePhasePending:
		reqLogger.Info("Validating UpgradeConfig")

		// Get current ClusterVersion
		cv, err := osd_cluster_upgrader.GetClusterVersion(r.client)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Build a Validator
		validator, err := r.validationBuilder.NewClient()
		if err != nil {
			return reconcile.Result{}, err
		}

		// Validate UpgradeConfig instance
		ok, err := validator.IsValidUpgradeConfig(instance, cv, reqLogger)
		if err != nil {
			reqLogger.Info("An error occurred while validating UpgradeConfig")
			metricsClient.UpdateMetricValidationFailed(instance.Name)
			return reconcile.Result{}, err
		}
		// If ok is false, desired version is <= current version.
		if !ok {
			reqLogger.Info("Desired version is <= current version. Not proceeding.")
			metricsClient.UpdateMetricValidationFailed(instance.Name)
			return reconcile.Result{}, nil
		}
		reqLogger.Info("UpgradeConfig validated.")
		metricsClient.UpdateMetricValidationSucceeded(instance.Name)

		cfm := r.configManagerBuilder.New(r.client, request.Namespace)
		cfg := &config{}
		err = cfm.Into(cfg)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Checking if cluster can commence upgrade.")
		ready := r.scheduler.IsReadyToUpgrade(instance, metricsClient, cfg.UpgradeWindow.TimeOut)
		if ready {
			upgrader, err := r.clusterUpgraderBuilder.NewClient(r.client, cfm, metricsClient, instance.Spec.Type)
			if err != nil {
				return reconcile.Result{}, err
			}

			now := time.Now()
			history.Phase = upgradev1alpha1.UpgradePhaseUpgrading
			history.StartTime = &metav1.Time{Time: now}
			instance.Status.History.SetHistory(history)
			err = r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info("Cluster is commencing upgrade.", "time", now)
			return r.upgradeCluster(upgrader, instance, reqLogger)
		} else {
			history.Phase = upgradev1alpha1.UpgradePhasePending
			instance.Status.History.SetHistory(history)
			err = r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
	case upgradev1alpha1.UpgradePhaseUpgrading:
		reqLogger.Info("Cluster detected as already upgrading.")
		cfm := r.configManagerBuilder.New(r.client, request.Namespace)
		upgrader, err := r.clusterUpgraderBuilder.NewClient(r.client, cfm, metricsClient, instance.Spec.Type)
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
	me := &multierror.Error{}

	phase, condition, err := upgrader.UpgradeCluster(uc, logger)
	me = multierror.Append(err, me)

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	history.Conditions = upgradev1alpha1.Conditions{*condition}
	history.Phase = phase
	if phase == upgradev1alpha1.UpgradePhaseUpgraded {
		history.CompleteTime = &metav1.Time{Time: time.Now()}
	}
	uc.Status.History.SetHistory(*history)
	err = r.client.Status().Update(context.TODO(), uc)
	me = multierror.Append(err, me)

	return reconcile.Result{}, me.ErrorOrNil()
}

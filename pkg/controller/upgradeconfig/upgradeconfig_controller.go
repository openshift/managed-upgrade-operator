package upgradeconfig

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	cub "github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader_builder"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	muocfg "github.com/openshift/managed-upgrade-operator/config"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	ucmgr "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
)

var (
	log = logf.Log.WithName("controller_upgradeconfig")
)

// Add creates a new UpgradeConfig Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	return &ReconcileUpgradeConfig{
		client:                 client,
		scheme:                 mgr.GetScheme(),
		metricsClientBuilder:   metrics.NewBuilder(),
		clusterUpgraderBuilder: cub.NewBuilder(),
		validationBuilder:      validation.NewBuilder(),
		configManagerBuilder:   configmanager.NewBuilder(),
		scheduler:              scheduler.NewScheduler(),
		cvClientBuilder:        cv.NewBuilder(),
		eventManagerBuilder:    eventmanager.NewBuilder(),
		ucMgrBuilder:           ucmgr.NewBuilder(),
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
	err = c.Watch(&source.Kind{Type: &upgradev1alpha1.UpgradeConfig{}}, &handler.EnqueueRequestForObject{}, StatusChangedPredicate, OSDUpgradePredicate)
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
	cvClientBuilder        cv.ClusterVersionBuilder
	eventManagerBuilder    eventmanager.EventManagerBuilder
	ucMgrBuilder           ucmgr.UpgradeConfigManagerBuilder
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

	// Initialise event manager
	eventClient, err := r.eventManagerBuilder.NewManager(r.client)
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
			metricsClient.ResetMetrics()
			reqLogger.Info("Reset all the metrics due to no upgrade config present.")
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
		cvClient := r.cvClientBuilder.New(r.client)
		clusterVersion, err := cvClient.GetClusterVersion()
		if err != nil {
			return reconcile.Result{}, err
		}

		// Build a Validator
		validator, err := r.validationBuilder.NewClient()
		if err != nil {
			return reconcile.Result{}, err
		}

		// Validate UpgradeConfig instance
		validatorResult, err := validator.IsValidUpgradeConfig(instance, clusterVersion, reqLogger)
		if err != nil {
			reqLogger.Info("An error occurred while validating UpgradeConfig")
			return reconcile.Result{}, err
		}
		if !validatorResult.IsValid {
			reqLogger.Info(validatorResult.Message)

			ValidationCondition := history.Conditions.GetCondition(upgradev1alpha1.UpgradeValidated)
			if ValidationCondition != nil {
				//update
				ValidationCondition.Status = "False"
				//msg = validatorResult.Message, reason
				ValidationCondition.Message = validatorResult.Message
				ValidationCondition.Reason = "Upgrade config validation failed"
				history.Conditions.SetCondition(*ValidationCondition)
				// instance.Status.History = history
				instance.Status.History.SetHistory(history)
			} else {
				//new, msg, reason
				newCond := upgradev1alpha1.UpgradeCondition{
					Type: upgradev1alpha1.UpgradeValidated, Status: "False",
					Message: validatorResult.Message, Reason: "Upgrade config validation failed"}
				history.Conditions.SetCondition(newCond)
				instance.Status.History.SetHistory(history)
			}

			err := r.client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}

			// need to set metrics outside controller -> collector
			// metricsClient.UpdateMetricValidationFailed(instance.Name)

			return reconcile.Result{}, nil
		}

		// need to set metrics outside controller -> collector
		metricsClient.UpdateMetricValidationSucceeded(instance.Name)

		if !validatorResult.IsAvailableUpdate {
			reqLogger.Info(validatorResult.Message)
			return reconcile.Result{}, nil
		}
		reqLogger.Info("UpgradeConfig validated and confirmed for upgrade.")

		cfm := r.configManagerBuilder.New(r.client, request.Namespace)
		cfg := &config{}
		err = cfm.Into(cfg)
		if err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Checking if cluster can commence upgrade.")
		schedulerResult := r.scheduler.IsReadyToUpgrade(instance, cfg.GetUpgradeWindowTimeOutDuration())
		if schedulerResult.IsReady {
			ucMgr, err := r.ucMgrBuilder.NewManager(r.client)
			if err != nil {
				return reconcile.Result{}, err
			}

			remoteChanged, err := ucMgr.Refresh()
			if err != nil {
				// If no config manager is configured, we don't need to enforce a kill switch
				if err != ucmgr.ErrNotConfigured {
					return reconcile.Result{}, err
				}
				reqLogger.Info("No UpgradeConfig manager configured, kill-switch ignored")
			}

			if remoteChanged {
				reqLogger.Info("The remote upgrade policy does not match the local upgrade config, applying the new upgrade policy")
				return reconcile.Result{}, nil
			}

			upgrader, err := r.clusterUpgraderBuilder.NewClient(r.client, cfm, metricsClient, eventClient, instance.Spec.Type)

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
			metricsClient.UpdateMetricUpgradeWindowNotBreached(instance.Name)
			return r.upgradeCluster(upgrader, instance, reqLogger)
		}

		history.Phase = upgradev1alpha1.UpgradePhasePending
		instance.Status.History.SetHistory(history)
		err = r.client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}

		if schedulerResult.IsBreached {
			// We are past the maximum allowed time to commence upgrading
			log.Error(nil, "field spec.upgradeAt cannot have backdated time")
			metricsClient.UpdateMetricUpgradeWindowBreached(instance.Name)
			return reconcile.Result{}, nil
		}

		// If we approach the time of the upgrade before the next reconcile,
		// reconcile closer to that point
		if schedulerResult.TimeUntilUpgrade.Seconds() > 0 &&
			schedulerResult.TimeUntilUpgrade < time.Duration(muocfg.SyncPeriodDefault) {
			return reconcile.Result{RequeueAfter: schedulerResult.TimeUntilUpgrade}, nil
		}

		return reconcile.Result{}, nil

	case upgradev1alpha1.UpgradePhaseUpgrading:
		reqLogger.Info("Cluster detected as already upgrading.")
		cfm := r.configManagerBuilder.New(r.client, request.Namespace)
		upgrader, err := r.clusterUpgraderBuilder.NewClient(r.client, cfm, metricsClient, eventClient, instance.Spec.Type)
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

	return reconcile.Result{RequeueAfter: 1 * time.Minute}, me.ErrorOrNil()
}

var OSDUpgradePredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		return isOsdUpgrade(e.MetaNew.GetName())
	},
	// Create is required to avoid reconciliation at controller initialisation.
	CreateFunc: func(e event.CreateEvent) bool {
		return isOsdUpgrade(e.Meta.GetName())
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return isOsdUpgrade(e.Meta.GetName())
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return isOsdUpgrade(e.Meta.GetName())
	},
}

func isOsdUpgrade(name string) bool {
	return name == ucmgr.UPGRADECONFIG_CR_NAME
}

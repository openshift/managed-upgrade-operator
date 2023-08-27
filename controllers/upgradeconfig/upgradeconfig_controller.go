package upgradeconfig

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	muocfg "github.com/openshift/managed-upgrade-operator/config"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	ucmgr "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	cub "github.com/openshift/managed-upgrade-operator/pkg/upgraders"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	log = logf.Log.WithName("controller_upgradeconfig")
)

// blank assignment to verify that ReconcileUpgradeConfig implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileUpgradeConfig{}

// ReconcileUpgradeConfig reconciles a UpgradeConfig object
type ReconcileUpgradeConfig struct {
	Client                 client.Client
	Scheme                 *runtime.Scheme
	MetricsClientBuilder   metrics.MetricsBuilder
	ClusterUpgraderBuilder cub.ClusterUpgraderBuilder
	ValidationBuilder      validation.ValidationBuilder
	ConfigManagerBuilder   configmanager.ConfigManagerBuilder
	Scheduler              scheduler.Scheduler
	CvClientBuilder        cv.ClusterVersionBuilder
	EventManagerBuilder    eventmanager.EventManagerBuilder
	UcMgrBuilder           ucmgr.UpgradeConfigManagerBuilder
}

// Reconcile reads that state of the cluster for a UpgradeConfig object and makes changes based on the state read
// and what is in the UpgradeConfig.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileUpgradeConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling UpgradeConfig")

	// Initialise metrics
	metricsClient, err := r.MetricsClientBuilder.NewClient(r.Client)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Initialise event manager
	eventClient, err := r.EventManagerBuilder.NewManager(r.Client)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Fetch the UpgradeConfig instance
	instance := &upgradev1alpha1.UpgradeConfig{}
	err = r.Client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			metricsClient.ResetEphemeralMetrics()
			reqLogger.Info("Reset metrics due to no upgrade config present.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Get current ClusterVersion
	cvClient := r.CvClientBuilder.New(r.Client)
	clusterVersion, err := cvClient.GetClusterVersion()
	if err != nil {
		return reconcile.Result{}, err
	}

	history := instance.Status.History.GetHistory(instance.Spec.Desired.Version)
	if history == nil {
		upgraded, err := cvClient.HasUpgradeCommenced(instance)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("could not tell if cluster was upgrading: %v", err)
		}
		if upgraded {
			// If CVO is currently set to the version of the UC, then we need to be in an Upgrading phase, at minimum.
			// We also won't know the actual start time, so picking now will just have to do
			history = &upgradev1alpha1.UpgradeHistory{
				Version:   instance.Spec.Desired.Version,
				Phase:     upgradev1alpha1.UpgradePhaseUpgrading,
				StartTime: &metav1.Time{Time: time.Now()}}
		} else {
			history = &upgradev1alpha1.UpgradeHistory{Version: instance.Spec.Desired.Version, Phase: upgradev1alpha1.UpgradePhaseNew}
		}
		history.Conditions = upgradev1alpha1.NewConditions()
		instance.Status.History = append([]upgradev1alpha1.UpgradeHistory{*history}, instance.Status.History...)
		err = r.Client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	status := history.Phase
	reqLogger.Info("Current cluster status", "status", status)
	switch status {
	case upgradev1alpha1.UpgradePhaseNew, upgradev1alpha1.UpgradePhasePending:
		reqLogger.Info("Validating UpgradeConfig")

		// Set up the ConfigManager and load MUO config
		target := muocfg.CMTarget{Namespace: request.Namespace}
		cmTarget, err := target.NewCMTarget()
		if err != nil {
			return reconcile.Result{}, err
		}

		cfm := r.ConfigManagerBuilder.New(r.Client, cmTarget)
		cfg := &config{}
		err = cfm.Into(cfg)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Build a Validator
		validator, err := r.ValidationBuilder.NewClient(cfm)
		if err != nil {
			return reconcile.Result{}, err
		}

		// Validate UpgradeConfig instance
		validatorResult, err := validator.IsValidUpgradeConfig(r.Client, instance, clusterVersion, reqLogger)
		if !validatorResult.IsValid || err != nil {
			reqLogger.Info(fmt.Sprintf("An error occurred while validating UpgradeConfig: %v", validatorResult.Message))
			metricsClient.UpdateMetricValidationFailed(instance.Name)
			return reconcile.Result{}, err
		}

		metricsClient.UpdateMetricValidationSucceeded(instance.Name)
		if !validatorResult.IsAvailableUpdate {
			reqLogger.Info(validatorResult.Message)
			return reconcile.Result{}, nil
		}
		reqLogger.Info("UpgradeConfig validated and confirmed for upgrade.")

		reqLogger.Info(fmt.Sprintf("Checking if cluster can commence %s upgrade.", instance.Spec.Type))
		schedulerResult := r.Scheduler.IsReadyToUpgrade(instance, cfg.GetUpgradeWindowTimeOutDuration())
		if schedulerResult.IsReady {
			ucMgr, err := r.UcMgrBuilder.NewManager(r.Client)
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
				reqLogger.Info("The cluster's upgrade policy has changed, so the operator will re-reconcile.")
				return reconcile.Result{}, nil
			}

			upgrader, err := r.ClusterUpgraderBuilder.NewClient(r.Client, cfm, metricsClient, eventClient, instance.Spec.Type)

			if err != nil {
				return reconcile.Result{}, err
			}

			now := time.Now()
			history.Phase = upgradev1alpha1.UpgradePhaseUpgrading
			history.StartTime = &metav1.Time{Time: now}
			history.Version = instance.Spec.Desired.Version

			instance.Status.History.SetHistory(*history)
			err = r.Client.Status().Update(context.TODO(), instance)
			if err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info(fmt.Sprintf("Cluster is commencing %s upgrade.", instance.Spec.Type), "time", now)
			return r.upgradeCluster(upgrader, instance, reqLogger)
		}

		history.Phase = upgradev1alpha1.UpgradePhasePending
		instance.Status.History.SetHistory(*history)
		err = r.Client.Status().Update(context.TODO(), instance)
		if err != nil {
			return reconcile.Result{}, err
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

		target := muocfg.CMTarget{Namespace: request.Namespace}
		cmTarget, err := target.NewCMTarget()
		if err != nil {
			return reconcile.Result{}, err
		}

		cfm := r.ConfigManagerBuilder.New(r.Client, cmTarget)
		upgrader, err := r.ClusterUpgraderBuilder.NewClient(r.Client, cfm, metricsClient, eventClient, instance.Spec.Type)
		if err != nil {
			return reconcile.Result{}, err
		}
		return r.upgradeCluster(upgrader, instance, reqLogger)
	case upgradev1alpha1.UpgradePhaseUpgraded:
		reqLogger.Info("Cluster is already upgraded")
		err = reportUpgradeMetrics(metricsClient, instance.Name, instance.Spec.Desired.Version, history.StartTime.Time, history.CompleteTime.Time)
		return reconcile.Result{}, err
	case upgradev1alpha1.UpgradePhaseFailed:
		reqLogger.Info("Cluster has failed to upgrade")
		return reconcile.Result{}, nil
	default:
		reqLogger.Info("Unknown status")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileUpgradeConfig) upgradeCluster(upgrader cub.ClusterUpgrader, uc *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (reconcile.Result, error) {
	me := &multierror.Error{}

	phase, err := upgrader.UpgradeCluster(context.TODO(), uc, logger)
	me = multierror.Append(err, me)

	history := uc.Status.History.GetHistory(uc.Spec.Desired.Version)
	history.Phase = phase
	if phase == upgradev1alpha1.UpgradePhaseUpgraded {
		history.CompleteTime = &metav1.Time{Time: time.Now()}
	}
	uc.Status.History.SetHistory(*history)
	err = r.Client.Status().Update(context.TODO(), uc)
	me = multierror.Append(err, me)

	return reconcile.Result{RequeueAfter: 1 * time.Minute}, me.ErrorOrNil()
}

// reportUpgradeMetrics updates prometheus with statistics from the latest upgrade
func reportUpgradeMetrics(metricsClient metrics.Metrics, name string, version string, upgradeStart time.Time, upgradeEnd time.Time) error {
	upgradeAlerts, err := metricsClient.AlertsFromUpgrade(upgradeStart, upgradeEnd)
	if err != nil {
		return err
	}

	metricsClient.UpdateMetricUpgradeResult(name, version, upgradeAlerts)
	return nil
}

// ManagedUpgradePredicate is used for managing predicates of the UpgradeConfig
func ManagedUpgradePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isManagedUpgrade(e.ObjectNew.GetName())
		},
		// Create is required to avoid reconciliation at controller initialisation.
		CreateFunc: func(e event.CreateEvent) bool {
			return isManagedUpgrade(e.Object.GetName())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isManagedUpgrade(e.Object.GetName())
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isManagedUpgrade(e.Object.GetName())
		},
	}
}

func isManagedUpgrade(name string) bool {
	return name == ucmgr.UPGRADECONFIG_CR_NAME
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileUpgradeConfig) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&upgradev1alpha1.UpgradeConfig{}).
		WithEventFilter(StatusChangedPredicate()).
		WithEventFilter(ManagedUpgradePredicate()).
		Complete(r)
}

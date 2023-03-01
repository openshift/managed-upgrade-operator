package collector

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

const (
	// .spec
	helpUpgradeAtTimestamp  = "Unix Timestamp indicating when the upgrade will execute"
	helpPdbTimeoutTimestamp = "Int indicating when the value of PDB timeout in minutes"

	// .status
	helpStartTime    = "Timestamp of when an upgrade starts"
	helpCompleteTime = "Timestamp of when an upgrade completes entirely"

	// .status.conditions[]
	helpSendStartedNotificationTimestamp       = "Unix Timestamp indicating time of start upgrade notification event"
	helpPreHealthCheckTimestamp                = "Unix Timestamp indicating time of cluster health check"
	helpExtDepAvailabilityTimestamp            = "Unix Timestamp indicating time of external dependency availability check"
	helpScaleUpExtraNodesTimestamp             = "Unix Timestamp indicating time of additional compute added"
	helpControlPlaneMaintWindowTimestamp       = "Unix Timestamp indicating start time of control plane maintenance"
	helpCommenceUpgradeTimestamp               = "Unix Timestamp indicating start time of upgrade"
	helpControlPlaneUpgradeCompleteTimestamp   = "Unix Timestamp indicating completion of upgrade upgrade"
	helpRemoveControlPlaneMaintWindowTimestamp = "Unix Timestamp indicating removal of control plane maintenance window"
	helpWorkersWindowTimestamp                 = "Unix Timestamp indicating start time of workers maintenance" //nolint:gosec
	helpAllWorkerNodesUpgradedTimestamp        = "Unix Timestamp indicating all worker nodes have upgraded"
	helpRemoveExtraScaledNodesTimestamp        = "Unix Timestamp indicating time of addtional compute removed"
	helpRemoveMaintWindow                      = "Unix Timestamp indicating end of workers maintenace"
	helpPostClusterHealthCheck                 = "Unix Timestamp indicating time of post cluster health check"
	helpSendCompletedNotificationTimestamp     = "Unix Timestamp indicating time of complete upgrade notification event"

	// Error handling for failed scrapes
	helpCollectorFailed = "An error occurred during scape of metrics"
)

// ManagedOSMetrics holds fields that contain upgrade metrics required to
// report during an upgrade. These metrics will be consistent regardless of the
// the upgrade.spec.type.
type ManagedOSMetrics struct {
	// .spec
	upgradeAt  *prometheus.Desc
	pdbTimeout *prometheus.Desc

	// .status
	startTime    *prometheus.Desc
	completeTime *prometheus.Desc

	// .status.conditions[]
	sendStartedNotification   *prometheus.Desc
	preHealthCheck            *prometheus.Desc
	extDepAvailCheck          *prometheus.Desc
	scaleUpExtraNodes         *prometheus.Desc
	controlPlaneMaintWindow   *prometheus.Desc
	commenceUpgrade           *prometheus.Desc
	controlPlaneUpgraded      *prometheus.Desc
	removeControlPlaneMaint   *prometheus.Desc
	workersMaintWindow        *prometheus.Desc
	allWorkerNodesUpgraded    *prometheus.Desc
	removeExtraScaledNodes    *prometheus.Desc
	removeMaintWindow         *prometheus.Desc
	postClusterHealthCheck    *prometheus.Desc
	sendCompletedNotification *prometheus.Desc
}

// UpgradeCollector is implementing prometheus.Collector interface.
// All these metrics will need to be registered at bootstrap via main.go
// as we can not all MustRegister every Reconcile.
type UpgradeCollector struct {
	upgradeConfigManager upgradeconfigmanager.UpgradeConfigManager
	cvClient             cv.ClusterVersion
	managedMetrics       *ManagedOSMetrics
}

// Consturct a new UpgradeCollector and return to caller.
func NewUpgradeCollector(c client.Client) (prometheus.Collector, error) {
	upgradeConfigManager, err := upgradeconfigmanager.NewBuilder().NewManager(c)
	if err != nil {
		return nil, err
	}

	managedMetrics := bootstrapMetrics()

	return &UpgradeCollector{
		upgradeConfigManager,
		cv.NewCVClient(c),
		managedMetrics,
	}, nil
}

func bootstrapMetrics() *ManagedOSMetrics {
	return &ManagedOSMetrics{
		startTime: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "start_timestamp"),
			helpStartTime,
			[]string{
				keyVersion,
				keyDesiredVersion,
				keyPhase,
			}, nil),
		completeTime: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "complete_timestamp"),
			helpCompleteTime,
			[]string{
				keyVersion,
				keyDesiredVersion,
				keyPhase,
			}, nil),
		upgradeAt: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "scheduled"),
			helpUpgradeAtTimestamp,
			[]string{
				keyVersion,
				keyDesiredVersion,
				keyPhase,
			}, nil),
		pdbTimeout: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "pdb_timeout_minutes"),
			helpPdbTimeoutTimestamp,
			[]string{
				keyVersion,
				keyDesiredVersion,
			}, nil),
		sendStartedNotification: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "notification_start_timestamp"),
			helpSendStartedNotificationTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		preHealthCheck: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "health_check_timestamp"),
			helpPreHealthCheckTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		extDepAvailCheck: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "external_dep_check_timestamp"),
			helpExtDepAvailabilityTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		scaleUpExtraNodes: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "capacity_added_timestamp"),
			helpScaleUpExtraNodesTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		controlPlaneMaintWindow: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "control_plane_maint_start_timestamp"),
			helpControlPlaneMaintWindowTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		commenceUpgrade: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "control_plane_upgrade_start_timestamp"),
			helpCommenceUpgradeTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		controlPlaneUpgraded: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "control_plane_completion_timestamp"),
			helpControlPlaneUpgradeCompleteTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		removeControlPlaneMaint: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "control_plane_maint_removed_timestamp"),
			helpRemoveControlPlaneMaintWindowTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		workersMaintWindow: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "workers_maint_start_timestamp"),
			helpWorkersWindowTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		allWorkerNodesUpgraded: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "workers_upgraded_timestamp"),
			helpAllWorkerNodesUpgradedTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		removeExtraScaledNodes: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "capacity_removed_timestamp"),
			helpRemoveExtraScaledNodesTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		removeMaintWindow: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "workers_maint_removed_timestamp"),
			helpRemoveMaintWindow,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		postClusterHealthCheck: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "post_upgrade_healthcheck_timestamp"),
			helpPostClusterHealthCheck,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
		sendCompletedNotification: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCondition, "notification_complete_timestamp"),
			helpSendCompletedNotificationTimestamp,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
				keyDesiredVersion,
				keyCondition,
			}, nil),
	}
}

// Describe implements the prometheus.Collector interface.
func (uc *UpgradeCollector) Describe(ch chan<- *prometheus.Desc) {
	// .spec
	ch <- uc.managedMetrics.upgradeAt
	ch <- uc.managedMetrics.pdbTimeout

	// .status
	ch <- uc.managedMetrics.startTime
	ch <- uc.managedMetrics.completeTime

	// .status.conditions[]
	ch <- uc.managedMetrics.sendStartedNotification
	ch <- uc.managedMetrics.preHealthCheck
	ch <- uc.managedMetrics.extDepAvailCheck
	ch <- uc.managedMetrics.scaleUpExtraNodes
	ch <- uc.managedMetrics.controlPlaneMaintWindow
	ch <- uc.managedMetrics.commenceUpgrade
	ch <- uc.managedMetrics.controlPlaneUpgraded
	ch <- uc.managedMetrics.removeControlPlaneMaint
	ch <- uc.managedMetrics.workersMaintWindow
	ch <- uc.managedMetrics.allWorkerNodesUpgraded
	ch <- uc.managedMetrics.removeExtraScaledNodes
	ch <- uc.managedMetrics.removeMaintWindow
	ch <- uc.managedMetrics.postClusterHealthCheck
	ch <- uc.managedMetrics.sendCompletedNotification
}

// Collect is method required to implement the prometheus.Collector(prometheus/client_golang/prometheus/collector.go) interface.
func (uc *UpgradeCollector) Collect(ch chan<- prometheus.Metric) {
	err := uc.collectUpgradeConditions(ch)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(
			prometheus.NewDesc(prometheus.BuildFQName(MetricsNamespace, subSystemCollector, "scrape_failed"),
				helpCollectorFailed, nil, nil),
			err)
	}
}

// collectUpgradeMetrics reviews the Status of the current UpgradeConfig and
// writes metrics based on whats available within the status.
func (uc *UpgradeCollector) collectUpgradeConditions(ch chan<- prometheus.Metric) error {
	upgradeConfig, err := uc.upgradeConfigManager.Get()
	if err != nil {
		if err == upgradeconfigmanager.ErrUpgradeConfigNotFound {
			return nil
		}
		return fmt.Errorf("unable to find UpgradeConfig: %v", err)
	}

	clusterVersion, err := uc.cvClient.GetClusterVersion()
	if err != nil {
		return err
	}
	cvVersion, err := cv.GetCurrentVersion(clusterVersion)
	if err != nil {
		return err
	}

	// Control plane has upgraded but metrics should retain appropriate
	// version -> desired.version for designated upgrade
	if upgradeConfig.Spec.Desired.Version == cvVersion {
		cvVersion, err = cv.GetCurrentVersionMinusOne(clusterVersion)
		if err != nil {
			return err
		}
	}

	// metrics that should collected regardless if upgrade has started
	if err = uc.collectSpec(upgradeConfig, cvVersion, ch); err != nil {
		return err
	}

	h := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	if h == nil {
		return fmt.Errorf("no upgrade history yet")
	}

	if err = uc.collectStatus(upgradeConfig, cvVersion, ch); err != nil {
		return err
	}

	// Collect metrics based on observing availble conditions in the target
	// versions upgrade history.
	for _, c := range h.Conditions {
		c := c
		switch c.Type {
		case upgradev1alpha1.SendStartedNotification:
			collectCondition(&c, uc.managedMetrics.sendStartedNotification, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.UpgradePreHealthCheck:
			collectCondition(&c, uc.managedMetrics.preHealthCheck, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.ExtDepAvailabilityCheck:
			collectCondition(&c, uc.managedMetrics.extDepAvailCheck, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.UpgradeScaleUpExtraNodes:
			collectCondition(&c, uc.managedMetrics.scaleUpExtraNodes, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.ControlPlaneMaintWindow:
			collectCondition(&c, uc.managedMetrics.controlPlaneMaintWindow, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.CommenceUpgrade:
			collectCondition(&c, uc.managedMetrics.commenceUpgrade, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.ControlPlaneUpgraded:
			collectCondition(&c, uc.managedMetrics.controlPlaneUpgraded, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.RemoveControlPlaneMaintWindow:
			collectCondition(&c, uc.managedMetrics.removeControlPlaneMaint, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.WorkersMaintWindow:
			collectCondition(&c, uc.managedMetrics.workersMaintWindow, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.AllWorkerNodesUpgraded:
			collectCondition(&c, uc.managedMetrics.allWorkerNodesUpgraded, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.RemoveExtraScaledNodes:
			collectCondition(&c, uc.managedMetrics.removeExtraScaledNodes, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.RemoveMaintWindow:
			collectCondition(&c, uc.managedMetrics.removeMaintWindow, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.PostClusterHealthCheck:
			collectCondition(&c, uc.managedMetrics.postClusterHealthCheck, upgradeConfig, cvVersion, ch)
		case upgradev1alpha1.SendCompletedNotification:
			collectCondition(&c, uc.managedMetrics.sendCompletedNotification, upgradeConfig, cvVersion, ch)
		}
	}
	return nil
}

func collectCondition(c *upgradev1alpha1.UpgradeCondition, promDesc *prometheus.Desc, ucfg *upgradev1alpha1.UpgradeConfig, cvV string, ch chan<- prometheus.Metric) {
	switch c.Status {
	case corev1.ConditionTrue:
		ch <- prometheus.MustNewConstMetric(
			promDesc,
			prometheus.GaugeValue,
			float64(c.CompleteTime.Unix()),
			ucfg.ObjectMeta.Name,
			cvV,
			ucfg.Spec.Desired.Version,
			string(corev1.ConditionTrue),
		)
	case corev1.ConditionFalse:
		ch <- prometheus.MustNewConstMetric(
			promDesc,
			prometheus.GaugeValue,
			float64(c.StartTime.Unix()),
			ucfg.ObjectMeta.Name,
			cvV,
			ucfg.Spec.Desired.Version,
			string(corev1.ConditionFalse),
		)
	case corev1.ConditionUnknown:
		ch <- prometheus.MustNewConstMetric(
			promDesc,
			prometheus.GaugeValue,
			float64(c.StartTime.Unix()),
			ucfg.ObjectMeta.Name,
			cvV,
			ucfg.Spec.Desired.Version,
			string(corev1.ConditionUnknown),
		)
	}
}

func (uc *UpgradeCollector) collectSpec(ucfg *upgradev1alpha1.UpgradeConfig, cvV string, ch chan<- prometheus.Metric) error {
	h := ucfg.Status.History.GetHistory(ucfg.Spec.Desired.Version)
	if h == nil {
		return fmt.Errorf("not able to fetch upgrade history")
	}
	upgradeTime, err := time.Parse(time.RFC3339, ucfg.Spec.UpgradeAt)
	if err != nil {
		return err
	}

	ch <- prometheus.MustNewConstMetric(
		uc.managedMetrics.upgradeAt,
		prometheus.GaugeValue,
		float64(upgradeTime.Unix()),
		cvV,
		ucfg.Spec.Desired.Version,
		string(h.Phase),
	)

	ch <- prometheus.MustNewConstMetric(
		uc.managedMetrics.pdbTimeout,
		prometheus.GaugeValue,
		float64(ucfg.Spec.PDBForceDrainTimeout),
		cvV,
		ucfg.Spec.Desired.Version,
	)
	return nil
}

func (uc *UpgradeCollector) collectStatus(ucfg *upgradev1alpha1.UpgradeConfig, cvV string, ch chan<- prometheus.Metric) error {
	h := ucfg.Status.History.GetHistory(ucfg.Spec.Desired.Version)
	if h == nil {
		return fmt.Errorf("not able to fetch upgrade history")
	}
	if h.StartTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.managedMetrics.startTime,
			prometheus.GaugeValue,
			float64(h.StartTime.Unix()),
			cvV,
			ucfg.Spec.Desired.Version,
			string(h.Phase),
		)
	}

	if h.CompleteTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.managedMetrics.completeTime,
			prometheus.GaugeValue,
			float64(h.CompleteTime.Unix()),
			cvV,
			ucfg.Spec.Desired.Version,
			string(h.Phase),
		)
	}
	return nil
}

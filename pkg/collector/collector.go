package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

const (
	// Core
	helpUpgradeState               = "Timestamp of upgrade state execution"
	helpConfigInvalid              = "Boolean indicating UpgradeConfig has failed validation"
	helpUpgradeHealthCheckFailed   = "Boolean indicating cluster health check has failed"
	helpScalingFailed              = "Boolean indicating failure to scale workers"
	helpClusterVerificationTimeout = "Boolean indicating cluster verification has failed"
	helpControlPlaneTimeout        = "Boolean indicating if the control plane upgrade timed out"
	helpWorkerTimeout              = "Boolean indicating if the worker upgrade timed out"
	helpNodeDrainFailed            = "Boolean indicating if a force node drain has failed"
	helpUpgradeWindowBreached      = "Boolean indicating if a the upgrade window has been breached"
	helpCollectorFailed            = "An error occurred during scape of metrics"

	// OSD
	helpNotificationEventSentFailed = "Boolean indicating if a the upgrade notification event has failed"
)

// CoreUpgradeMetrics holds fields that contain upgrade metrics required to
// report during an upgrade. These metrics will be consistent regardless of the
// the upgrade.spec.type.
type CoreUpgradeMetrics struct {
	upgradeState              *prometheus.Desc
	configInvalid             *prometheus.Desc
	upgradeHealthCheckFailed  *prometheus.Desc
	scalingFailed             *prometheus.Desc
	clusterVerificationFailed *prometheus.Desc
	controlPlaneTimeout       *prometheus.Desc
	workerTimeout             *prometheus.Desc
	nodeDrainFailed           *prometheus.Desc
	upgradeWindowBreached     *prometheus.Desc
}

// OSDUpgradeMetrics holds metrics specific to the OSD upgrader requirements.
type OSDUpgradeMetrics struct {
	// TODO: This is called outside of Reconciles
	//providerSyncFailed    *prometheus.Desc
	notificationEventSentFailed *prometheus.Desc
}

// AROUpgradeMetics holds metrics specific to the OSD upgrader requirements.
type AROUpgradeMetrics struct {
}

// UpgradeCollector is implementing prometheus.Collector interface.
// All these metrics will need to be registered at bootstrap via main.go
// as we can not all MustRegister every Reconcile.
// TODO/Idea [dofinn]: Deregister metrics != .spec.type at runtime?
type UpgradeCollector struct {
	upgradeConfigManager ucm.UpgradeConfigManager
	cvClient             cv.ClusterVersion
	coreUpgradeMetrics   *CoreUpgradeMetrics
	osdUpgradeMetrics    *OSDUpgradeMetrics
	aroUpgradeMetrics    *AROUpgradeMetrics
}

// Consturct a new UpgradeCollector and return to caller.
func NewUpgradeCollector(c client.Client) (prometheus.Collector, error) {
	upgradeConfigManager, err := ucm.NewBuilder().NewManager(c)
	if err != nil {
		return nil, err
	}

	coreMetrics := &CoreUpgradeMetrics{
		upgradeState: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "state_timestamp"),
			helpUpgradeState,
			[]string{
				keyVersion,
				keyPhase,
			}, nil),
		configInvalid: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgradeConfig, "invalid"),
			helpConfigInvalid,
			[]string{
				keyUpgradeConfigName,
			}, nil),
		upgradeHealthCheckFailed: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCluster, "health_check_failed"),
			helpUpgradeHealthCheckFailed,
			[]string{
				keyUpgradeConfigName,
				keyState,
			}, nil),
		scalingFailed: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "scaling_failed"),
			helpScalingFailed,
			[]string{
				keyDimension,
			}, nil),
		clusterVerificationFailed: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemCluster, "verification_failed"),
			helpClusterVerificationTimeout,
			[]string{
				keyUpgradeConfigName,
				keyPhase,
			}, nil),
		controlPlaneTimeout: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "control_plane_timeout"),
			helpControlPlaneTimeout,
			[]string{
				keyUpgradeConfigName,
				keyPhase,
			}, nil),
		workerTimeout: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "worker_timeout"),
			helpWorkerTimeout,
			[]string{
				keyUpgradeConfigName,
				keyVersion,
			}, nil),
		nodeDrainFailed: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "node_drain_failed"),
			helpNodeDrainFailed,
			[]string{
				keyNodeName,
			}, nil),
		upgradeWindowBreached: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "window_breached"),
			helpUpgradeWindowBreached,
			[]string{
				keyUpgradeConfigName,
			}, nil),
	}

	//	osdMetrics := &OSDUpgradeMetrics{
	//		providerSyncFailed: prometheus.NewDesc(
	//			prometheus.BuildFQName(MetricsNamespace, subSystemUpgradeConfig, "sync_failed"),
	//			"Boolean indicating if the sync with UpgradeConfig provider failed",
	//			[]string{
	//				keyUpgradeConfigName,
	//			}, nil),
	//	}
	osdMetrics := &OSDUpgradeMetrics{
		notificationEventSentFailed: prometheus.NewDesc(
			prometheus.BuildFQName(MetricsNamespace, subSystemNotification, "event_sent_failed"),
			helpNotificationEventSentFailed,
			[]string{
				keyUpgradeConfigName,
				keyState,
				keyVersion,
			}, nil),
	}
	aroMetrics := &AROUpgradeMetrics{}

	return &UpgradeCollector{
		upgradeConfigManager,
		cv.NewCVClient(c),
		coreMetrics,
		osdMetrics,
		aroMetrics,
	}, nil
}

// Describe implements the prometheus.Collector interface.
func (uc *UpgradeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- uc.coreUpgradeMetrics.upgradeState
	ch <- uc.coreUpgradeMetrics.configInvalid
	ch <- uc.coreUpgradeMetrics.upgradeHealthCheckFailed
	ch <- uc.coreUpgradeMetrics.scalingFailed
	ch <- uc.coreUpgradeMetrics.clusterVerificationFailed
	ch <- uc.coreUpgradeMetrics.controlPlaneTimeout
	ch <- uc.coreUpgradeMetrics.workerTimeout
	ch <- uc.coreUpgradeMetrics.nodeDrainFailed
	ch <- uc.coreUpgradeMetrics.upgradeWindowBreached

	ch <- uc.osdUpgradeMetrics.notificationEventSentFailed
}

// Collect is method required to implement the prometheus.Collector(prometheus/client_golang/prometheus/collector.go) interface.
func (uc *UpgradeCollector) Collect(ch chan<- prometheus.Metric) {
	err := uc.collectUpgradeMetrics(ch)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(
			prometheus.NewDesc(prometheus.BuildFQName(MetricsNamespace, subSystemCollector, "scrape_failed"),
				helpCollectorFailed, nil, nil),
			err)
	}
}

// collectUpgradeMetrics reviews the Status of the current UpgradeConfig and
// writes metrics based on whats available within the status.
func (uc *UpgradeCollector) collectUpgradeMetrics(ch chan<- prometheus.Metric) error {
	upgradeConfig, err := uc.upgradeConfigManager.Get()
	if err != nil {
		return err
	}

	// metrics that should always be collected
	if err = uc.collectUpgradeAtTimestamp(upgradeConfig, ch); err != nil {
		return err
	}
	uc.collectConfigValidation(upgradeConfig, ch)
	uc.collectNotificationEvent(upgradeConfig, ch)

	// metrics for "pending" upgrade
	if upgradeConfig.Status.History != nil {
		history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
		// map this out with case statements for phase
		if history != nil {
			uc.collectStartTime(upgradeConfig, history, ch)
			if history.Phase == upgradev1alpha1.UpgradePhaseUpgrading {
				uc.collectClusterHealthCheckFailed(upgradeConfig, history, ch)
				uc.collectUpgradeWindowBreach(upgradeConfig, ch)
			}

			clusterVersion, err := uc.cvClient.GetClusterVersion()
			if err != nil {
				return err
			}

			// metrics that should be recorded once the upgrade has started
			cvHistory := cv.GetHistory(clusterVersion, upgradeConfig.Spec.Desired.Version)
			if cvHistory != nil {
				uc.collectControlPlaneStartTime(upgradeConfig, cvHistory, ch)
				uc.collectControlPlaneTimeout(upgradeConfig, cvHistory, ch)
				uc.collectControlPlaneCompletionTime(upgradeConfig, cvHistory, ch)
				uc.collectNodeDrainFailed(upgradeConfig, history, ch)
				uc.collectWorkerStartTime(upgradeConfig, history, ch)
				uc.collectWorkerCompleteTime(upgradeConfig, history, ch)
				uc.collectClusterVerificationFailed(upgradeConfig, ch)
				uc.collectCompleteTime(upgradeConfig, history, ch)
				uc.collectWorkerTimeout(upgradeConfig, history, ch)
			}
		}
	}
	return nil
}

func (uc *UpgradeCollector) collectScalingFailed(upgradeConfig *upgradev1alpha1.UpgradeConfig, ch chan<- prometheus.Metric) {
	scalingFailed := upgradeConfig.Status.History[0].Scaling.Failed

	if scalingFailed {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.scalingFailed,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.Status.History[0].Scaling.Dimension,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.scalingFailed,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.Status.History[0].Scaling.Dimension,
	)
}

func (uc *UpgradeCollector) collectConfigValidation(upgradeConfig *upgradev1alpha1.UpgradeConfig, ch chan<- prometheus.Metric) {
	configInvalid := upgradeConfig.Status.ConfigInvalid

	if configInvalid {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.configInvalid,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.configInvalid,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
	)
}

func (uc *UpgradeCollector) collectUpgradeWindowBreach(upgradeConfig *upgradev1alpha1.UpgradeConfig, ch chan<- prometheus.Metric) {
	windowBreached := upgradeConfig.Status.History[0].WindowBreached

	if windowBreached {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeWindowBreached,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.upgradeWindowBreached,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
	)
}

func (uc *UpgradeCollector) collectNotificationEvent(upgradeConfig *upgradev1alpha1.UpgradeConfig, ch chan<- prometheus.Metric) {
	eventSentFailed := upgradeConfig.Status.NotificationEvent.Failed

	if eventSentFailed {
		ch <- prometheus.MustNewConstMetric(
			uc.osdUpgradeMetrics.notificationEventSentFailed,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
			string(upgradeConfig.Status.NotificationEvent.State),
			upgradeConfig.Spec.Desired.Version,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.osdUpgradeMetrics.notificationEventSentFailed,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
		string(upgradeConfig.Status.NotificationEvent.State),
		upgradeConfig.Spec.Desired.Version,
	)
}

func (uc *UpgradeCollector) collectClusterVerificationFailed(upgradeConfig *upgradev1alpha1.UpgradeConfig, ch chan<- prometheus.Metric) {
	clusterVerificatinFailed := upgradeConfig.Status.History[0].ClusterVerificationFailed

	if clusterVerificatinFailed {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.clusterVerificationFailed,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
			ValuePostUpgrade,
		)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.clusterVerificationFailed,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
		ValuePostUpgrade,
	)
}

func (uc *UpgradeCollector) collectUpgradeAtTimestamp(upgradeConfig *upgradev1alpha1.UpgradeConfig, ch chan<- prometheus.Metric) error {

	if upgradeConfig.Spec.UpgradeAt != "" {
		upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
		if err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(upgradeTime.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valuePending,
		)
	}

	return nil
}

func (uc *UpgradeCollector) collectClusterHealthCheckFailed(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {
	healthCheckFailed := upgradeConfig.Status.History[0].HealthCheck.Failed

	if healthCheckFailed {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeHealthCheckFailed,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
			upgradeConfig.Status.History[0].HealthCheck.State,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.upgradeHealthCheckFailed,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
		upgradeConfig.Status.History[0].HealthCheck.State,
	)
}

func (uc *UpgradeCollector) collectStartTime(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {
	if h.StartTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(h.StartTime.Time.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valueUpgrading,
		)
	}
}

func (uc *UpgradeCollector) collectWorkerStartTime(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {
	if h.WorkerStartTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(h.WorkerStartTime.Time.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valueWorkersStarted,
		)
	}
}

func (uc *UpgradeCollector) collectWorkerCompleteTime(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {
	if h.WorkerCompleteTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(h.WorkerCompleteTime.Time.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valueWorkersCompleted,
		)
	}
}

func (uc *UpgradeCollector) collectWorkerTimeout(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {

	workerTimeout := upgradeConfig.Status.History[0].WorkerTimeout

	if workerTimeout {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.workerTimeout,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
			upgradeConfig.Spec.Desired.Version,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.workerTimeout,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
		upgradeConfig.Spec.Desired.Version,
	)
}

func (uc *UpgradeCollector) collectNodeDrainFailed(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {

	drainFailed := upgradeConfig.Status.History[0].NodeDrain.Failed

	if drainFailed {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.nodeDrainFailed,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.Status.History[0].NodeDrain.Name,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.nodeDrainFailed,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.Status.History[0].NodeDrain.Name,
	)
}
func (uc *UpgradeCollector) collectCompleteTime(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *upgradev1alpha1.UpgradeHistory, ch chan<- prometheus.Metric) {
	if h.CompleteTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(h.CompleteTime.Time.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valueCompleted,
		)
	}
}

func (uc *UpgradeCollector) collectControlPlaneTimeout(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *v1.UpdateHistory, ch chan<- prometheus.Metric) {

	controlPlaneTimeout := upgradeConfig.Status.History[0].ControlPlaneTimeout

	if controlPlaneTimeout {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.controlPlaneTimeout,
			prometheus.GaugeValue,
			float64(1),
			upgradeConfig.ObjectMeta.Name,
			valueControlPlaneStarted,
		)
		return
	}

	ch <- prometheus.MustNewConstMetric(
		uc.coreUpgradeMetrics.controlPlaneTimeout,
		prometheus.GaugeValue,
		float64(0),
		upgradeConfig.ObjectMeta.Name,
		valueControlPlaneStarted,
	)
}

func (uc *UpgradeCollector) collectControlPlaneStartTime(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *v1.UpdateHistory, ch chan<- prometheus.Metric) {
	if &h.StartedTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(h.StartedTime.Time.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valueControlPlaneStarted,
		)
	}
}

func (uc *UpgradeCollector) collectControlPlaneCompletionTime(upgradeConfig *upgradev1alpha1.UpgradeConfig, h *v1.UpdateHistory, ch chan<- prometheus.Metric) {
	if h.CompletionTime != nil {
		ch <- prometheus.MustNewConstMetric(
			uc.coreUpgradeMetrics.upgradeState,
			prometheus.GaugeValue,
			float64(h.CompletionTime.Time.Unix()),
			upgradeConfig.Spec.Desired.Version,
			valueControlPlaneCompleted,
		)
	}
}

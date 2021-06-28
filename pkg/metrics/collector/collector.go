package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

// UpgradeMetrics holds fields that contain upgrade metrics required to
// report during an upgrade.
type UpgradeMetrics struct {
	upgradeState *prometheus.Desc
}

// UpgradeCollector is implementing prometheus.Collector interface.
type UpgradeCollector struct {
	upgradeConfigManager ucm.UpgradeConfigManager
	cvClient             cv.ClusterVersion
	upgradeMetrics       *UpgradeMetrics
}

// NewUpgradeCollector constructs a new UpgradeCollector and return to caller.
func NewUpgradeCollector(c client.Client) (prometheus.Collector, error) {
	upgradeConfigManager, err := ucm.NewBuilder().NewManager(c)
	if err != nil {
		return nil, err
	}

	uMetrics := &UpgradeMetrics{
		upgradeState: prometheus.NewDesc(
			prometheus.BuildFQName(metrics.Namespace, metrics.Subsystem, "state_timestamp"),
			"Timestamps of upgrade state",
			[]string{
				metrics.VersionLabel,
				metrics.StateLabel,
			}, nil),
	}

	return &UpgradeCollector{
		upgradeConfigManager,
		cv.NewCVClient(c),
		uMetrics,
	}, nil
}

// Describe implements the prometheus.Collector interface.
func (uc *UpgradeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- uc.upgradeMetrics.upgradeState
}

// Collect is method required to implement the prometheus.Collector(prometheus/client_golang/prometheus/collector.go) interface.
func (uc *UpgradeCollector) Collect(ch chan<- prometheus.Metric) {
	uc.collectUpgradeMetrics(ch)
}

func (uc *UpgradeCollector) collectUpgradeMetrics(ch chan<- prometheus.Metric) {
	upgradeConfig, err := uc.upgradeConfigManager.Get()
	if err != nil {
		return
	}

	upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		return
	}

	// Set scheduled state value
	ch <- prometheus.MustNewConstMetric(
		uc.upgradeMetrics.upgradeState,
		prometheus.GaugeValue,
		float64(upgradeTime.Unix()),
		upgradeConfig.Spec.Desired.Version,
		metrics.ScheduledStateValue,
	)

	if upgradeConfig.Status.History != nil {
		history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
		if history != nil {
			if history.StartTime != nil {
				// Set started state value
				ch <- prometheus.MustNewConstMetric(
					uc.upgradeMetrics.upgradeState,
					prometheus.GaugeValue,
					float64(history.StartTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
					metrics.StartedStateValue,
				)
			}
			if history.WorkerStartTime != nil {
				// Set workers started state value
				ch <- prometheus.MustNewConstMetric(
					uc.upgradeMetrics.upgradeState,
					prometheus.GaugeValue,
					float64(history.WorkerStartTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
					metrics.WorkersStartedStateValue,
				)
			}
			if history.WorkerCompleteTime != nil {
				// Set workers completed state value
				ch <- prometheus.MustNewConstMetric(
					uc.upgradeMetrics.upgradeState,
					prometheus.GaugeValue,
					float64(history.WorkerCompleteTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
					metrics.WorkersCompletedStateValue,
				)
			}
			if history.CompleteTime != nil {
				// Set finished state value
				ch <- prometheus.MustNewConstMetric(
					uc.upgradeMetrics.upgradeState,
					prometheus.GaugeValue,
					float64(history.CompleteTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
					metrics.FinishedStateValue,
				)
			}

			clusterVersion, err := uc.cvClient.GetClusterVersion()
			if err != nil {
				return
			}

			cvHistory := cv.GetHistory(clusterVersion, upgradeConfig.Spec.Desired.Version)
			if cvHistory != nil {
				// Set control plane started state value
				ch <- prometheus.MustNewConstMetric(
					uc.upgradeMetrics.upgradeState,
					prometheus.GaugeValue,
					float64(cvHistory.StartedTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
					metrics.ControlPlaneStartedStateValue,
				)

				if cvHistory.CompletionTime != nil {
					// Set control plane completed state value
					ch <- prometheus.MustNewConstMetric(
						uc.upgradeMetrics.upgradeState,
						prometheus.GaugeValue,
						float64(cvHistory.CompletionTime.Time.Unix()),
						upgradeConfig.Spec.Desired.Version,
						metrics.ControlPlaneCompletedStateValue,
					)
				}
			}
		}
	}
}

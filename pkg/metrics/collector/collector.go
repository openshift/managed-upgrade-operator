package collector

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

// UpgradeMetrics holds fields that contain upgrade metrics required to
// report during an upgrade.
type UpgradeMetrics struct {
	upgradeState     *prometheus.Desc
	upgradeCollector *prometheus.Desc
}

// UpgradeCollector is implementing prometheus.Collector interface.
type UpgradeCollector struct {
	upgradeConfigManager ucm.UpgradeConfigManager
	cvClient             cv.ClusterVersion
	upgradeMetrics       *UpgradeMetrics
}

// Consturct a new UpgradeCollector and return to caller.
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
		upgradeCollector: prometheus.NewDesc(
			prometheus.BuildFQName(metrics.Namespace, metrics.Subsystem, "collector_failing"),
			"Collector failing indicator",
			[]string{}, nil),
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
	ch <- uc.upgradeMetrics.upgradeCollector
}

// Collect is method required to implement the prometheus.Collector(prometheus/client_golang/prometheus/collector.go) interface.
func (uc *UpgradeCollector) Collect(ch chan<- prometheus.Metric) {
	// Set this to not failing by default. Explicity set it so the metrics
	// is always reported to avoid gaps in graphes etc from "missing metrics"
	ch <- prometheus.MustNewConstMetric(
		uc.upgradeMetrics.upgradeCollector,
		prometheus.GaugeValue,
		float64(0),
	)
	if err := uc.collectUpgradeMetrics(ch); err != nil {
		// If the collector is erroring trying to get metrics, set a metric
		// advising of this condition.
		ch <- prometheus.MustNewConstMetric(
			uc.upgradeMetrics.upgradeCollector,
			prometheus.GaugeValue,
			float64(1),
		)
	}
}

func (uc *UpgradeCollector) collectUpgradeMetrics(ch chan<- prometheus.Metric) error {
	upgradeConfig, err := uc.upgradeConfigManager.Get()
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return upgradeconfigmanager.ErrRetrievingUpgradeConfigs

	}

	upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		return err
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
				return err
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
	return nil
}

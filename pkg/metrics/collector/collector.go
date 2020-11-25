package collector

import (
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	ucm "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager"
)

var (
	UpgradeValidationFailedDesc = prometheus.NewDesc(
		"validate_result",
		"Failed to validate the upgrade config",
		[]string{
			"version",
		}, nil)

	UpgradeSchedulingDesc = prometheus.NewDesc(
		"upgrade_at",
		"The desired start time of the upgrade",
		[]string{
			"version",
		}, nil)

	UpgradeStartDesc = prometheus.NewDesc(
		"upgrade_start",
		"The actual start time of the managed upgrade",
		[]string{
			"version",
		}, nil)

	UpgradeControlPlaneStartDesc = prometheus.NewDesc(
		"upgrade_control_plane_start",
		"The start time of control plane upgrades",
		[]string{
			"version",
		}, nil)

	UpgradeControlPlaneCompletionDesc = prometheus.NewDesc(
		"upgrade_control_plane_completion",
		"The completion time of control plane upgrades",
		[]string{
			"version",
		}, nil)

	UpgradeWorkerStartDesc = prometheus.NewDesc(
		"upgrade_worker_start",
		"The start time of the worker upgrades",
		[]string{
			"version",
		}, nil)

	UpgradeWorkerCompleteDesc = prometheus.NewDesc(
		"upgrade_worker_completion",
		"The completion time of the worker upgrades",
		[]string{
			"version",
		}, nil)

	UpgradeCompleteDesc = prometheus.NewDesc(
		"upgrade_complete",
		"The completion time of the managed upgrade",
		[]string{
			"version",
		}, nil)
)

// UpgradeCollector is implementing prometheus.Collector interface.
type UpgradeCollector struct {
	upgradeConfigManager ucm.UpgradeConfigManager
	cvClient             cv.ClusterVersion
}

func NewUpgradeCollector(c client.Client) (prometheus.Collector, error) {
	upgradeConfigManager, err := ucm.NewBuilder().NewManager(c)
	if err != nil {
		return nil, err
	}
	return &UpgradeCollector{
		upgradeConfigManager,
		cv.NewCVClient(c),
	}, nil
}

// Describe implements the prometheus.Collector interface.
func (uc *UpgradeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- UpgradeSchedulingDesc
	ch <- UpgradeStartDesc
	ch <- UpgradeControlPlaneStartDesc
	ch <- UpgradeControlPlaneCompletionDesc
	ch <- UpgradeWorkerStartDesc
	ch <- UpgradeWorkerCompleteDesc
	ch <- UpgradeCompleteDesc
}

// Collect is method required to implement the prometheus.Collector(prometheus/client_golang/prometheus/collector.go) interface.
func (uc *UpgradeCollector) Collect(ch chan<- prometheus.Metric) {
	uc.collectUpgradeMetrics(ch)
}

var history upgradev1alpha1.UpgradeHistory

func (uc *UpgradeCollector) collectUpgradeMetrics(ch chan<- prometheus.Metric) {
	upgradeConfig, err := uc.upgradeConfigManager.Get()
	if err != nil {
		return
	}

	upgradeTime, err := time.Parse(time.RFC3339, upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(
		UpgradeSchedulingDesc,
		prometheus.GaugeValue,
		float64(upgradeTime.Unix()),
		upgradeConfig.Spec.Desired.Version,
	)

	if upgradeConfig.Status.History != nil {
		history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
		if history != nil {
			if history.Conditions.GetCondition(upgradev1alpha1.UpgradeValidated) != nil {
				ch <- prometheus.MustNewConstMetric(
					UpgradeValidationFailedDesc,
					prometheus.GaugeValue,
					float64(1),
					upgradeConfig.Spec.Desired.Version,
				)
			}

			if history.StartTime != nil {
				ch <- prometheus.MustNewConstMetric(
					UpgradeStartDesc,
					prometheus.GaugeValue,
					float64(history.StartTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
				)
			}
			if history.WorkerStartTime != nil {
				ch <- prometheus.MustNewConstMetric(
					UpgradeWorkerStartDesc,
					prometheus.GaugeValue,
					float64(history.WorkerStartTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
				)
			}
			if history.WorkerCompleteTime != nil {
				ch <- prometheus.MustNewConstMetric(
					UpgradeWorkerCompleteDesc,
					prometheus.GaugeValue,
					float64(history.WorkerCompleteTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
				)
			}
			if history.CompleteTime != nil {
				ch <- prometheus.MustNewConstMetric(
					UpgradeCompleteDesc,
					prometheus.GaugeValue,
					float64(history.CompleteTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
				)
			}

			clusterVersion, err := uc.cvClient.GetClusterVersion()
			if err != nil {
				return
			}

			cvHistory := cv.GetHistory(clusterVersion, upgradeConfig.Spec.Desired.Version)
			if cvHistory != nil {
				ch <- prometheus.MustNewConstMetric(
					UpgradeControlPlaneStartDesc,
					prometheus.GaugeValue,
					float64(cvHistory.StartedTime.Time.Unix()),
					upgradeConfig.Spec.Desired.Version,
				)

				if cvHistory.CompletionTime != nil {
					ch <- prometheus.MustNewConstMetric(
						UpgradeControlPlaneCompletionDesc,
						prometheus.GaugeValue,
						float64(cvHistory.CompletionTime.Time.Unix()),
						upgradeConfig.Spec.Desired.Version,
					)
				}
			}
		}
	}
}

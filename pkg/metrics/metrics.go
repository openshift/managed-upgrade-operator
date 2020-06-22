package metrics

import (
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsTag 	= "upgradeoperator"
	nameLabel 	= "upgradeconfig_name"
)

var (
	metricValidationFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name: "upgradeconfig_validation_failed",
		Help: "Failed to validate the upgrade config",
	}, []string{nameLabel})
	metricClusterCheckFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name: "cluster_check_failed",
		Help: "Failed on the cluster check step",
	}, []string{nameLabel})
	metricUpgradeStartTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name: "upgrade_start_timestamp",
		Help: "Timestamp for the real upgrade process is started",
	}, []string{nameLabel})
	metricControlPlaneUpgradeEndTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name: "controlplane_upgrade_end_timestamp",
		Help: "Timestamp for the control plane upgrade is finished",
	}, []string{nameLabel})
	metricNodeUpgradeEndTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name: "node_upgrade_end_timestamp",
		Help: "Timestamp for the node upgrade is finished",
	}, []string{nameLabel})
	metricPostVerificationFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name: "post_verification_failed",
		Help: "Failed on the post upgrade verification step",
	}, []string{nameLabel})
)

func init() {
	metrics.Registry.MustRegister(metricValidationFailed)
	metrics.Registry.MustRegister(metricClusterCheckFailed)
	metrics.Registry.MustRegister(metricUpgradeStartTime)
	metrics.Registry.MustRegister(metricControlPlaneUpgradeEndTime)
	metrics.Registry.MustRegister(metricNodeUpgradeEndTime)
	metrics.Registry.MustRegister(metricPostVerificationFailed)
}

func UpdateMetricValidationFailed(x int, upgradeconfig string) {
	metricValidationFailed.With(prometheus.Labels{
		nameLabel: upgradeconfig}).Set(
			float64(x))
}

func UpdateMetricClusterCheckFailed(x int, upgradeconfig string) {
	metricClusterCheckFailed.With(prometheus.Labels{
		nameLabel: upgradeconfig}).Set(
			float64(x))
}

func UpdateMetricUpgradeStartTime(time time.Time, upgradeconfig string) {
	metricUpgradeStartTime.With(prometheus.Labels{
		nameLabel: upgradeconfig}).Set(
			float64(time.Unix()))
}

func UpdateMetricControlPlaneEndTime(time time.Time, upgradeconfig string) {
	metricControlPlaneUpgradeEndTime.With(prometheus.Labels{
		nameLabel: upgradeconfig}).Set(
			float64(time.Unix()))
}

func UpdateMetricNodeUpgradeEndTime(time time.Time, upgradeconfig string) {
	metricNodeUpgradeEndTime.With(prometheus.Labels{
		nameLabel: upgradeconfig}).Set(
		float64(time.Unix()))
}

func UpdateMetricPostVerificationFailed(x int, upgradeconfig string) {
	metricPostVerificationFailed.With(prometheus.Labels{
		nameLabel: upgradeconfig}).Set(
			float64(x))
}
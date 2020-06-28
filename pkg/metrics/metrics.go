package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/prometheus/client_golang/prometheus"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"strings"
	"time"
)

const (
	metricsTag   = "upgradeoperator"
	nameLabel    = "upgradeconfig_name"
	versionLabel = "version"
)

type Metrics interface {
	UpdateMetricValidationFailed(string)
	UpdateMetricValidationSucceeded(string)
	UpdateMetricClusterCheckFailed(string)
	UpdateMetricClusterCheckSucceeded(string)
	UpdateMetricUpgradeStartTime(time.Time, string, string)
	UpdateMetricControlPlaneEndTime(time.Time, string, string)
	UpdateMetricNodeUpgradeEndTime(time.Time, string, string)
	UpdateMetricClusterVerificationFailed(string)
	UpdateMetricClusterVerificationSucceeded(string)
	IsMetricUpgradeStartTimeSet(upgradeConfigName string, version string) (bool, error)
	IsMetricControlPlaneEndTimeSet(upgradeConfigName string, version string) (bool, error)
	IsMetricNodeUpgradeEndTimeSet(upgradeConfigName string, version string) (bool, error)
	Query(query string) (*AlertResponse, error)
}

type MetricsBuilder struct{}

func (mb *MetricsBuilder) NewClient(c client.Client) (Metrics, error) {
	promUrl, err := getPromUrl(c)
	if err != nil {
		return nil, err
	}

	token, err := getPrometheusToken(c)
	if err != nil {
		return nil, err
	}

	return &Counter{
		promBaseUrl: *promUrl,
		promClient: http.Client{
			Transport: &prometheusRoundTripper{
				token: *token,
			},
		},
	}, nil
}

type prometheusRoundTripper struct {
	token string
}

func (prt *prometheusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", "Bearer "+prt.token)
	transport := http.Transport{}
	return transport.RoundTrip(req)
}

type Counter struct {
	promClient  http.Client
	promBaseUrl string
}

var (
	metricValidationFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "upgradeconfig_validation_failed",
		Help:      "Failed to validate the upgrade config",
	}, []string{nameLabel})
	metricClusterCheckFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "cluster_check_failed",
		Help:      "Failed on the cluster check step",
	}, []string{nameLabel})
	metricUpgradeStartTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "upgrade_start_timestamp",
		Help:      "Timestamp for the real upgrade process is started",
	}, []string{nameLabel})
	metricControlPlaneUpgradeEndTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "controlplane_upgrade_end_timestamp",
		Help:      "Timestamp for the control plane upgrade is finished",
	}, []string{nameLabel})
	metricNodeUpgradeEndTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "node_upgrade_end_timestamp",
		Help:      "Timestamp for the node upgrade is finished",
	}, []string{nameLabel})
	metricClusterVerificationFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "cluster_verification_failed",
		Help:      "Failed on the cluster upgrade verification step",
	}, []string{nameLabel})
)

func init() {
	metrics.Registry.MustRegister(metricValidationFailed)
	metrics.Registry.MustRegister(metricClusterCheckFailed)
	metrics.Registry.MustRegister(metricUpgradeStartTime)
	metrics.Registry.MustRegister(metricControlPlaneUpgradeEndTime)
	metrics.Registry.MustRegister(metricNodeUpgradeEndTime)
	metrics.Registry.MustRegister(metricClusterVerificationFailed)
}

func (c *Counter) UpdateMetricValidationFailed(upgradeConfigName string) {
	metricValidationFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) UpdateMetricValidationSucceeded(upgradeConfigName string) {
	metricValidationFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(0))
}

func (c *Counter) UpdateMetricClusterCheckFailed(upgradeConfigName string) {
	metricClusterCheckFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) UpdateMetricClusterCheckSucceeded(upgradeConfigName string) {
	metricClusterCheckFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(0))
}

func (c *Counter) UpdateMetricUpgradeStartTime(time time.Time, upgradeConfigName string, version string) {
	metricUpgradeStartTime.With(prometheus.Labels{
		versionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(time.Unix()))
}

func (c *Counter) UpdateMetricControlPlaneEndTime(time time.Time, upgradeConfigName string, version string) {
	metricControlPlaneUpgradeEndTime.With(prometheus.Labels{
		versionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(time.Unix()))
}

func (c *Counter) IsMetricUpgradeStartTimeSet(upgradeConfigName string, version string) (bool, error) {
	cpMetrics, err := c.Query(fmt.Sprintf("upgrade_start_timestamp{%s=\"%s\",%s=\"%s\"", nameLabel, upgradeConfigName, versionLabel, version))
	if err != nil {
		return false, err
	}

	if len(cpMetrics.Data.Result) > 0 {
		return true, nil
	}

	return false, nil
}

func (c *Counter) IsMetricControlPlaneEndTimeSet(upgradeConfigName string, version string) (bool, error) {
	cpMetrics, err := c.Query(fmt.Sprintf("controlplane_upgrade_end_timestamp{%s=\"%s\",%s=\"%s\"", nameLabel, upgradeConfigName, versionLabel, version))
	if err != nil {
		return false, err
	}

	if len(cpMetrics.Data.Result) > 0 {
		return true, nil
	}

	return false, nil
}

func (c *Counter) IsMetricNodeUpgradeEndTimeSet(upgradeConfigName string, version string) (bool, error) {
	cpMetrics, err := c.Query(fmt.Sprintf("node_upgrade_end_timestamp{%s=\"%s\",%s=\"%s\"", nameLabel, upgradeConfigName, versionLabel, version))
	if err != nil {
		return false, err
	}

	if len(cpMetrics.Data.Result) > 0 {
		return true, nil
	}

	return false, nil
}

func (c *Counter) UpdateMetricNodeUpgradeEndTime(time time.Time, upgradeConfigName string, version string) {
	metricNodeUpgradeEndTime.With(prometheus.Labels{
		versionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(time.Unix()))
}

func (c *Counter) UpdateMetricClusterVerificationFailed(upgradeConfigName string) {
	metricClusterVerificationFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) UpdateMetricClusterVerificationSucceeded(upgradeConfigName string) {
	metricClusterVerificationFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(0))
}

func getPromUrl(c client.Client) (*string, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-monitoring", Name: "prometheus-k8s"}, route)
	if err != nil {
		return nil, err
	}

	return &route.Spec.Host, nil
}

func (c *Counter) Query(query string) (*AlertResponse, error) {
	req, err := http.NewRequest("GET", "https://"+c.promBaseUrl+"/api/v1/query", nil)
	if err != nil {
		return nil, fmt.Errorf("Could not query Prometheus: %s", err)
	}

	q := req.URL.Query()
	q.Add("query", query)
	req.URL.RawQuery = q.Encode()
	resp, err := c.promClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error when querying Prometheus: %s", err)
	}

	result := &AlertResponse{}
	err = json.Unmarshal(body, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func getPrometheusToken(c client.Client) (*string, error) {
	sa := &corev1.ServiceAccount{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-monitoring", Name: "prometheus-k8s"}, sa)
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch prometheus-k8s service account: %s", err)
	}

	tokenSecret := ""
	for _, secret := range sa.Secrets {
		if strings.HasPrefix(secret.Name, "prometheus-k8s-token") {
			tokenSecret = secret.Name
		}
	}
	if len(tokenSecret) == 0 {
		return nil, fmt.Errorf("Failed to find token secret for prommetheus-k8s SA")
	}

	secret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-monitoring", Name: tokenSecret}, secret)
	if err != nil {
		return nil, fmt.Errorf("Unable to fetch secret %s: %s", tokenSecret, err)
	}

	token := secret.Data[corev1.ServiceAccountTokenKey]
	stringToken := string(token)

	return &stringToken, nil
}

type AlertResponse struct {
	Status string    `json:"status"`
	Data   AlertData `json:"data"`
}

type AlertData struct {
	Result []AlertResult `json:"result"`
}

type AlertResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value"`
}

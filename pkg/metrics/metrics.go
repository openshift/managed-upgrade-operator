package metrics

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/managed-upgrade-operator/config"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	eventLabel = "event"
	metricsTag = "upgradeoperator"
	nameLabel  = "upgradeconfig_name"
	nodeLabel  = "node_name"

	Namespace = "upgradeoperator"
	Subsystem = "upgrade"

	StateLabel   = "state"
	VersionLabel = "version"

	ScheduledStateValue             = "scheduled"
	StartedStateValue               = "started"
	FinishedStateValue              = "finished"
	ControlPlaneStartedStateValue   = "control_plane_started"
	ControlPlaneCompletedStateValue = "control_plane_completed"
	WorkersStartedStateValue        = "workers_started"
	WorkersCompletedStateValue      = "workers_completed"

	MonitoringNS              = "openshift-monitoring"
	MonitoringCAConfigMapName = "serving-certs-ca-bundle"
	MonitoringConfigField     = "service-ca.crt"
	promApp                   = "prometheus-k8s"
	clusterSVCSuffix          = ".svc.cluster.local"
)

//go:generate mockgen -destination=mocks/metrics.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/metrics Metrics
type Metrics interface {
	UpdateMetricValidationFailed(string)
	UpdateMetricValidationSucceeded(string)
	UpdateMetricClusterCheckFailed(string)
	UpdateMetricClusterCheckSucceeded(string)
	UpdateMetricScalingFailed(string)
	UpdateMetricScalingSucceeded(string)
	UpdateMetricUpgradeWindowNotBreached(string)
	UpdateMetricUpgradeConfigSynced(string)
	ResetMetricUpgradeConfigSynced(string)
	UpdateMetricUpgradeWindowBreached(string)
	UpdateMetricUpgradeControlPlaneTimeout(string, string)
	ResetMetricUpgradeControlPlaneTimeout(string, string)
	UpdateMetricUpgradeWorkerTimeout(string, string)
	ResetMetricUpgradeWorkerTimeout(string, string)
	UpdateMetricNodeDrainFailed(string)
	ResetMetricNodeDrainFailed(string)
	ResetAllMetricNodeDrainFailed()
	ResetFailureMetrics()
	ResetAllMetrics()
	UpdateMetricNotificationEventSent(string, string, string)
	IsAlertFiring(alert string, checkedNS, ignoredNS []string) (bool, error)
	IsMetricNotificationEventSentSet(upgradeConfigName string, event string, version string) (bool, error)
	IsClusterVersionAtVersion(version string) (bool, error)
	Query(query string) (*AlertResponse, error)
}

//go:generate mockgen -destination=mocks/metrics_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/metrics MetricsBuilder
type MetricsBuilder interface {
	NewClient(c client.Client) (Metrics, error)
}

func NewBuilder() MetricsBuilder {
	return &metricsBuilder{}
}

type metricsBuilder struct{}

func (mb *metricsBuilder) NewClient(c client.Client) (Metrics, error) {
	promEndpoint, err := Endpoint(c, MonitoringNS, promApp, "web")
	if err != nil {
		return nil, err
	}

	token, err := prometheusToken(c)
	if err != nil {
		return nil, err
	}

	useRoutes := config.UseRoutes()
	tlsConfig := &tls.Config{}

	if !useRoutes {
		tlsConfig, err = TLSConfig(c)
		if err != nil {
			return nil, err
		}
	}

	return &Counter{
		promEndpoint: promEndpoint,
		promClient: http.Client{
			Transport: &prometheusRoundTripper{
				token: *token,
				tls:   tlsConfig,
			},
		},
	}, nil
}

type prometheusRoundTripper struct {
	token string
	tls   *tls.Config
}

func TLSConfig(c client.Client) (*tls.Config, error) {
	var tls tls.Config

	cfgMap := &corev1.ConfigMap{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: MonitoringCAConfigMapName, Namespace: MonitoringNS}, cfgMap)
	if err != nil {
		return &tls, err
	}

	ca := cfgMap.Data[MonitoringConfigField]

	if ca == "" {
		return &tls, fmt.Errorf("monitoring service CA returned nil")
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if ok := rootCAs.AppendCertsFromPEM([]byte(ca)); !ok {
		return &tls, fmt.Errorf("failed to append certs")
	}

	tls.RootCAs = rootCAs

	return &tls, nil
}

func (prt *prometheusRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", "Bearer "+prt.token)
	transport := http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
		TLSClientConfig:     prt.tls,
	}
	return transport.RoundTrip(req)
}

type Counter struct {
	promClient   http.Client
	promEndpoint string
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
	metricScalingFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "scaling_failed",
		Help:      "Failed to scale up extra workers",
	}, []string{nameLabel})
	metricUpgradeWindowBreached = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "upgrade_window_breached",
		Help:      "Failed to commence upgrade during the upgrade window",
	}, []string{nameLabel})
	metricUpgradeConfigSynced = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "upgradeconfig_synced",
		Help:      "UpgradeConfig has not been synced in time",
	}, []string{nameLabel})
	metricUpgradeControlPlaneTimeout = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "controlplane_timeout",
		Help:      "Control plane upgrade timeout",
	}, []string{nameLabel, VersionLabel})
	metricUpgradeWorkerTimeout = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "worker_timeout",
		Help:      "Worker nodes upgrade timeout",
	}, []string{nameLabel, VersionLabel})
	metricNodeDrainFailed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "node_drain_timeout",
		Help:      "Node cannot be drained successfully in time.",
	}, []string{nodeLabel})
	metricUpgradeNotification = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Subsystem: metricsTag,
		Name:      "upgrade_notification",
		Help:      "Notification event raised",
	}, []string{nameLabel, eventLabel, VersionLabel})

	metricsList = []*prometheus.GaugeVec{
		metricValidationFailed,
		metricClusterCheckFailed,
		metricScalingFailed,
		metricUpgradeWindowBreached,
		metricUpgradeConfigSynced,
		metricUpgradeControlPlaneTimeout,
		metricUpgradeWorkerTimeout,
		metricNodeDrainFailed,
		metricUpgradeNotification,
	}
)

func init() {
	for _, m := range metricsList {
		metrics.Registry.MustRegister(m)
	}
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

func (c *Counter) UpdateMetricScalingFailed(upgradeConfigName string) {
	metricScalingFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) UpdateMetricScalingSucceeded(upgradeConfigName string) {
	metricScalingFailed.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(0))
}

func (c *Counter) UpdateMetricUpgradeConfigSynced(name string) {
	metricUpgradeConfigSynced.With(prometheus.Labels{nameLabel: name}).Set(float64(1))
}

func (c *Counter) ResetMetricUpgradeConfigSynced(name string) {
	metricUpgradeConfigSynced.With(prometheus.Labels{nameLabel: name}).Set(float64(0))
}

func (c *Counter) UpdateMetricUpgradeControlPlaneTimeout(upgradeConfigName, version string) {
	metricUpgradeControlPlaneTimeout.With(prometheus.Labels{
		VersionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) ResetMetricUpgradeControlPlaneTimeout(upgradeConfigName, version string) {
	metricUpgradeControlPlaneTimeout.With(prometheus.Labels{
		VersionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(0))
}

func (c *Counter) UpdateMetricUpgradeWorkerTimeout(upgradeConfigName, version string) {
	metricUpgradeWorkerTimeout.With(prometheus.Labels{
		VersionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) ResetMetricUpgradeWorkerTimeout(upgradeConfigName, version string) {
	metricUpgradeWorkerTimeout.With(prometheus.Labels{
		VersionLabel: version,
		nameLabel:    upgradeConfigName}).Set(
		float64(0))
}

func (c *Counter) UpdateMetricNodeDrainFailed(nodeName string) {
	metricNodeDrainFailed.With(prometheus.Labels{
		nodeLabel: nodeName}).Set(
		float64(1))
}

func (c *Counter) ResetMetricNodeDrainFailed(nodeName string) {
	metricNodeDrainFailed.With(prometheus.Labels{
		nodeLabel: nodeName}).Set(
		float64(0))
}

func (c *Counter) ResetAllMetricNodeDrainFailed() {
	metricNodeDrainFailed.Reset()
}

func (c *Counter) UpdateMetricUpgradeWindowNotBreached(upgradeConfigName string) {
	metricUpgradeWindowBreached.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(0))
}

func (c *Counter) UpdateMetricUpgradeWindowBreached(upgradeConfigName string) {
	metricUpgradeWindowBreached.With(prometheus.Labels{
		nameLabel: upgradeConfigName}).Set(
		float64(1))
}

func (c *Counter) UpdateMetricNotificationEventSent(upgradeConfigName string, event string, version string) {
	metricUpgradeNotification.With(prometheus.Labels{
		VersionLabel: version,
		eventLabel:   event,
		nameLabel:    upgradeConfigName}).Set(
		float64(1))
}

// ResetAllMetrics will reset all the metrics
func (c *Counter) ResetAllMetrics() {
	for _, m := range metricsList {
		m.Reset()
	}
}

// ResetFailureMetrics will reset the metric which indicates the upgrade failed
func (c *Counter) ResetFailureMetrics() {
	failureMetricsList := []*prometheus.GaugeVec{
		metricValidationFailed,
		metricClusterCheckFailed,
		metricScalingFailed,
		metricUpgradeControlPlaneTimeout,
		metricUpgradeWorkerTimeout,
		metricNodeDrainFailed,
	}
	for _, m := range failureMetricsList {
		m.Reset()
	}
}

func (c *Counter) IsMetricNotificationEventSentSet(upgradeConfigName string, event string, version string) (bool, error) {
	cpMetrics, err := c.Query(fmt.Sprintf("%s_upgrade_notification{%s=\"%s\",%s=\"%s\",%s=\"%s\"}", metricsTag, nameLabel, upgradeConfigName, eventLabel, event, VersionLabel, version))
	if err != nil {
		return false, err
	}

	if len(cpMetrics.Data.Result) > 0 {
		return true, nil
	}

	return false, nil
}

func (c *Counter) IsClusterVersionAtVersion(version string) (bool, error) {
	cpMetrics, err := c.Query(fmt.Sprintf("cluster_version{version=\"%s\",type=\"current\"}", version))
	if err != nil {
		return false, err
	}

	if len(cpMetrics.Data.Result) > 0 {
		return true, nil
	}

	return false, nil
}

func (c *Counter) IsAlertFiring(alert string, checkedNS, ignoredNS []string) (bool, error) {
	cpMetrics, err := c.Query(fmt.Sprintf(`ALERTS{alertstate="firing",alertname="%s",namespace=~"^$|%s",namespace!="%s"}`,
		alert, strings.Join(checkedNS, "|"), strings.Join(ignoredNS, "|")))

	if err != nil {
		return false, err
	}

	if len(cpMetrics.Data.Result) > 0 {
		return true, nil
	}
	return false, nil
}

// GetServiceEndpoint accepts a client,namespace,svcName and portName and attempts to retrive
// the services endpoint in the form of resolveable.service:portnumber.
func GetServiceEndpoint(c client.Client, namespace, svcName, portName string) (string, error) {
	svc := &corev1.Service{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: svcName}, svc)
	if err != nil {
		return "", err
	}

	host := fmt.Sprintf(svcName + "." + namespace + clusterSVCSuffix)
	var port string
	for _, p := range svc.Spec.Ports {
		if p.Name == portName {
			port = strconv.FormatInt(int64(p.Port), 10)
		}
	}
	endpoint := fmt.Sprint(host + ":" + port)
	return endpoint, nil
}

func isRunModeLocal() bool {
	return os.Getenv(k8sutil.ForceRunModeEnv) == string(k8sutil.LocalRunMode)
}

func Endpoint(c client.Client, namespace, appName, portName string) (string, error) {
	var endpoint string
	var err error

	runLocal := isRunModeLocal()
	useRoutes := config.UseRoutes()

	if !runLocal || runLocal && !useRoutes {
		endpoint, err = GetServiceEndpoint(c, namespace, appName, portName)
		if err != nil {
			return endpoint, err
		}
	} else {
		endpoint, err = getRouteEndpoint(c, appName)
		if err != nil {
			return endpoint, err
		}
	}

	return endpoint, nil
}

func getRouteEndpoint(c client.Client, appName string) (string, error) {
	route := &routev1.Route{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: MonitoringNS, Name: appName}, route)
	if err != nil {
		return "", err
	}

	return route.Spec.Host, nil
}

func (c *Counter) Query(query string) (*AlertResponse, error) {
	req, err := http.NewRequest("GET", "https://"+c.promEndpoint+"/api/v1/query", nil)
	if err != nil {
		return nil, fmt.Errorf("could not query prometheus: %s", err)
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
		return nil, fmt.Errorf("error when querying prometheus: %s", err)
	}

	result := &AlertResponse{}
	err = json.Unmarshal(body, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func prometheusToken(c client.Client) (*string, error) {
	sa := &corev1.ServiceAccount{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: MonitoringNS, Name: promApp}, sa)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch prometheus-k8s service account: %s", err)
	}

	tokenSecret := ""
	for _, secret := range sa.Secrets {
		if strings.HasPrefix(secret.Name, "prometheus-k8s-token") {
			tokenSecret = secret.Name
		}
	}
	if len(tokenSecret) == 0 {
		return nil, fmt.Errorf("failed to find token secret for prometheus-k8s SA")
	}

	secret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: MonitoringNS, Name: tokenSecret}, secret)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch secret %s: %s", tokenSecret, err)
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

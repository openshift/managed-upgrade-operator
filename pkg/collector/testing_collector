package collector

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

const (
	TEST_OPERATOR_NAMESPACE = "test-namespace"
	TEST_UPGRADECONFIG_CR   = "test-upgrade-config"
	TEST_UPGRADE_VERSION    = "4.4.4"
	TEST_UPGRADE_CHANNEL    = "stable-4.4"
	TEST_UPGRADE_TIME       = "2020-06-20T00:00:00Z"
	TEST_UPGRADE_PDB_TIME   = 60
	TEST_UPGRADE_TYPE       = "OSD"
)

var _ = Describe("Collector", func() {
	const (
		testCompletedUpdate configv1.UpdateState = "Completed"
	)

	var (
		upgradeConfig         upgradev1alpha1.UpgradeConfig
		nillTimeUpgradeConfig upgradev1alpha1.UpgradeConfig
		cv                    configv1.ClusterVersion
		nillTimeCV            configv1.ClusterVersion
		uCollector            prometheus.Collector

		mockKubeClient *mocks.MockClient
		mockCtrl       *gomock.Controller

		metricName string
		expected   string
		metadata   string

		testTime time.Time

		// Currently its required to playback these metrics. Its not possible to CollectAndGather with metric names
		// and given labels as matching is done strictly on metric name.
		expectedStatePending          string
		expectedCompleted             string
		expectedControlPlaneCompleted string
		expectedControlPlaneStarted   string
		expectedWorkersCompleted      string
		expectedWorkersStarted        string
		expectedUpgrading             string
	)

	BeforeEach(func() {
		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		uCollector, _ = NewUpgradeCollector(mockKubeClient)
		testTime = time.Now()

		upgradeConfig = upgradev1alpha1.UpgradeConfig{
			ObjectMeta: v1.ObjectMeta{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			},
			Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{
					Version: TEST_UPGRADE_VERSION,
					Channel: TEST_UPGRADE_CHANNEL,
				},
				UpgradeAt:            TEST_UPGRADE_TIME,
				PDBForceDrainTimeout: TEST_UPGRADE_PDB_TIME,
				Type:                 TEST_UPGRADE_TYPE,
			},
			Status: upgradev1alpha1.UpgradeConfigStatus{
				History: []upgradev1alpha1.UpgradeHistory{
					{
						Version:            upgradeConfig.Spec.Desired.Version,
						Phase:              upgradev1alpha1.UpgradePhaseUpgrading,
						StartTime:          &metav1.Time{Time: testTime},
						CompleteTime:       &metav1.Time{Time: testTime},
						WorkerStartTime:    &metav1.Time{Time: testTime},
						WorkerCompleteTime: &metav1.Time{Time: testTime},
						HealthCheck: upgradev1alpha1.HealthCheck{
							Failed: false,
							State:  ValuePreUpgrade,
						},
						Scaling: upgradev1alpha1.Scaling{
							Failed:    false,
							Dimension: "down",
						},
						ClusterVerificationFailed: false,
						ControlPlaneTimeout:       false,
						WorkerTimeout:             false,
						NodeDrain: upgradev1alpha1.Drain{
							Failed: false,
							Name:   "cool_node",
						},
						WindowBreached: false,
					},
				},
			},
		}
		nillTimeUpgradeConfig = upgradev1alpha1.UpgradeConfig{
			ObjectMeta: v1.ObjectMeta{
				Name:      TEST_UPGRADECONFIG_CR,
				Namespace: TEST_OPERATOR_NAMESPACE,
			},
			Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{
					Version: TEST_UPGRADE_VERSION,
					Channel: TEST_UPGRADE_CHANNEL,
				},
				PDBForceDrainTimeout: TEST_UPGRADE_PDB_TIME,
				Type:                 TEST_UPGRADE_TYPE,
			},
			Status: upgradev1alpha1.UpgradeConfigStatus{
				History: []upgradev1alpha1.UpgradeHistory{
					{
						Version: upgradeConfig.Spec.Desired.Version,
						Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
						HealthCheck: upgradev1alpha1.HealthCheck{
							Failed: false,
							State:  ValuePreUpgrade,
						},
						Scaling: upgradev1alpha1.Scaling{
							Failed:    false,
							Dimension: "down",
						},
						ClusterVerificationFailed: false,
						ControlPlaneTimeout:       false,
						WorkerTimeout:             false,
						NodeDrain: upgradev1alpha1.Drain{
							Failed: false,
							Name:   "cool_node",
						},
						WindowBreached: false,
					},
				},
			},
		}
		cv = configv1.ClusterVersion{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: upgradeConfig.Spec.Desired.Version},
				Channel:       upgradeConfig.Spec.Desired.Channel,
			},
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
				},
				History: []configv1.UpdateHistory{
					{
						State: testCompletedUpdate,
						StartedTime: v1.Time{
							Time: time.Now().UTC().Add(-60 * time.Minute),
						},
						CompletionTime: &v1.Time{
							Time: time.Now().UTC().Add(-60 * time.Minute),
						},
						Version:  "some bad version",
						Verified: false,
					},
					{
						State: testCompletedUpdate,
						StartedTime: v1.Time{
							Time: time.Now().UTC(),
						},
						CompletionTime: &v1.Time{
							Time: time.Now().UTC(),
						},
						Version:  upgradeConfig.Spec.Desired.Version,
						Verified: false,
					},
				},
			},
		}
		nillTimeCV = configv1.ClusterVersion{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: upgradeConfig.Spec.Desired.Version},
				Channel:       upgradeConfig.Spec.Desired.Channel,
			},
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
				},
				History: []configv1.UpdateHistory{
					{
						State: testCompletedUpdate,
						StartedTime: v1.Time{
							Time: time.Now().UTC(),
						},
						Version:  upgradeConfig.Spec.Desired.Version,
						Verified: false,
					},
				},
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("Collecting metrics of an upgrade", func() {
		Context("When the UpgradeConfig illustrates a succcessfully compelted upgrade", func() {
			It("will write a metric timestamp for upgradeAt time", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "state_timestamp")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpUpgradeState, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valuePending, keyVersion, upgradeConfig.Spec.Desired.Version, stringToUnixTime(upgradeConfig.Spec.UpgradeAt))
				expectedStatePending = expected
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			//	Context("UpgradeConfig validation fails", func() {
			//		It("will write a metric for configInvalid being true", func() {
			//			upgradeConfig.Status.ConfigInvalid = true
			//			metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgradeConfig, "invalid")
			//			metadata = fmt.Sprintf(`
			//			# HELP %s %s
			//			# TYPE %s gauge
			//			`, metricName, helpConfigInvalid, metricName)
			//			expected = fmt.Sprintf(`
			//			%s{%s="%s"} %f
			//			`, metricName, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(1))
			//			gomock.InOrder(
			//				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
			//			)
			//			err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
			//			Expect(err).To(BeNil())
			//		})
			//	})
			It("will write a metric for configInvalid not failing", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgradeConfig, "invalid")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpConfigInvalid, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s"} %f
				`, metricName, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric for configInvalid failing", func() {
				upgradeConfig.Status.ConfigInvalid = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgradeConfig, "invalid")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpConfigInvalid, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s"} %f
				`, metricName, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			//	Context("Event notification sending fails", func() {
			//		It("will write a metric for notification failure being true", func() {
			//			upgradeConfig.Status.NotificationEvent.Failed = true
			//			upgradeConfig.Status.NotificationEvent.State = string(notifier.StateStarted)
			//			metricName = prometheus.BuildFQName(MetricsNamespace, subSystemNotification, "event_sent_failed")
			//			metadata = fmt.Sprintf(`
			//			# HELP %s %s
			//			# TYPE %s gauge
			//			`, metricName, helpNotificationEventSentFailed, metricName)
			//			expected = fmt.Sprintf(`
			//			%s{%s="%s",%s="%s",%s="%s"} %f
			//			`, metricName, keyState, upgradeConfig.Status.NotificationEvent.State, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, keyVersion, TEST_UPGRADE_VERSION, float64(1))
			//			gomock.InOrder(
			//				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
			//			)
			//			err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
			//			Expect(err).To(BeNil())
			//		})
			//	})
			It("will write a metric for notification failure being false", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemNotification, "event_sent_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpNotificationEventSentFailed, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s",%s="%s"} %f
				`, metricName, keyState, upgradeConfig.Status.NotificationEvent.State, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, keyVersion, TEST_UPGRADE_VERSION, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric timestamp to indicate cluster phase timestamps", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "state_timestamp")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpUpgradeState, metricName)
				expectedUpgrading = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueUpgrading, keyVersion, upgradeConfig.Spec.Desired.Version, float64(upgradeConfig.Status.History[0].StartTime.Unix()))
				expectedCompleted = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueCompleted, keyVersion, upgradeConfig.Spec.Desired.Version, float64(upgradeConfig.Status.History[0].CompleteTime.Time.Unix()))
				expectedControlPlaneCompleted = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueControlPlaneCompleted, keyVersion, upgradeConfig.Spec.Desired.Version, float64(cv.Status.History[1].CompletionTime.Time.Unix()))
				expectedControlPlaneStarted = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueControlPlaneStarted, keyVersion, upgradeConfig.Spec.Desired.Version, float64(cv.Status.History[1].StartedTime.Time.Unix()))
				expectedWorkersCompleted = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueWorkersCompleted, keyVersion, upgradeConfig.Spec.Desired.Version, float64(upgradeConfig.Status.History[0].WorkerCompleteTime.Unix()))
				expectedWorkersStarted = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueWorkersStarted, keyVersion, upgradeConfig.Spec.Desired.Version, float64(upgradeConfig.Status.History[0].WorkerStartTime.Unix()))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector,
					strings.NewReader(metadata+expectedCompleted+expectedControlPlaneCompleted+expectedControlPlaneStarted+expectedStatePending+expectedUpgrading+expectedWorkersCompleted+expectedWorkersStarted), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate cluster health check did not fail", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemCluster, "health_check_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpUpgradeHealthCheckFailed, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyState, upgradeConfig.Status.History[0].HealthCheck.State, keyUpgradeConfigName, upgradeConfig.ObjectMeta.Name, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate cluster health check failed", func() {
				upgradeConfig.Status.History[0].HealthCheck.Failed = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemCluster, "health_check_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpUpgradeHealthCheckFailed, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyState, upgradeConfig.Status.History[0].HealthCheck.State, keyUpgradeConfigName, upgradeConfig.ObjectMeta.Name, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate upgrade window has not been breached", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "window_breached")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpUpgradeWindowBreached, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s"} %f
				`, metricName, keyUpgradeConfigName, upgradeConfig.ObjectMeta.Name, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate upgrade window has been breached", func() {
				upgradeConfig.Status.History[0].WindowBreached = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "window_breached")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpUpgradeWindowBreached, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s"} %f
				`, metricName, keyUpgradeConfigName, upgradeConfig.ObjectMeta.Name, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate node drains are not failing", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "node_drain_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpNodeDrainFailed, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s"} %f
				`, metricName, keyNodeName, upgradeConfig.Status.History[0].NodeDrain.Name, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate node drains are failing", func() {
				upgradeConfig.Status.History[0].NodeDrain.Failed = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "node_drain_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpNodeDrainFailed, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s"} %f
				`, metricName, keyNodeName, upgradeConfig.Status.History[0].NodeDrain.Name, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate worker timeout has not failed", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "worker_timeout")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpWorkerTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, keyVersion, upgradeConfig.Spec.Desired.Version, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate worker timeout has failed", func() {
				upgradeConfig.Status.History[0].WorkerTimeout = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "worker_timeout")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpWorkerTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, keyVersion, upgradeConfig.Spec.Desired.Version, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate cluster verification has not failed", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemCluster, "verification_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpClusterVerificationTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, ValuePostUpgrade, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate cluster verification has failed", func() {
				upgradeConfig.Status.History[0].ClusterVerificationFailed = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemCluster, "verification_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpClusterVerificationTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, ValuePostUpgrade, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate control plane has not timed out", func() {
				upgradeConfig.Status.History[0].ClusterVerificationFailed = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "control_plane_timeout")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpControlPlaneTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueControlPlaneStarted, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
			It("will write a metric to indicate control plane has timed out", func() {
				upgradeConfig.Status.History[0].ControlPlaneTimeout = true
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "control_plane_timeout")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpControlPlaneTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueControlPlaneStarted, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(1))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, upgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, cv).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
		})
		Context("When all non required time fields are nil", func() {
			It("will not write metrics for nil timestamps", func() {
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemUpgrade, "control_plane_timeout")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpControlPlaneTimeout, metricName)
				expected = fmt.Sprintf(`
				%s{%s="%s",%s="%s"} %f
				`, metricName, keyPhase, valueControlPlaneStarted, keyUpgradeConfigName, TEST_UPGRADECONFIG_CR, float64(0))
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, nillTimeUpgradeConfig).Return(nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, nillTimeCV).Return(nil),
				)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).To(BeNil())
			})
		})
		Context("When an error occurs", func() {
			It("An metric is returned indicating a failed scrape", func() {
				_ = os.Unsetenv("OPERATOR_NAMESPACE")
				metricName = prometheus.BuildFQName(MetricsNamespace, subSystemCollector, "scrape_failed")
				metadata = fmt.Sprintf(`
				# HELP %s %s
				# TYPE %s gauge
				`, metricName, helpCollectorFailed, metricName)
				expected = fmt.Sprintf(`
				%s
				`, metricName)
				err := promtestutil.CollectAndCompare(uCollector, strings.NewReader(metadata+expected), metricName)
				Expect(err).ToNot(BeNil())
			})
		})
	})
})

func stringToUnixTime(s string) float64 {
	t, _ := time.Parse(time.RFC3339, s)
	return float64(t.Unix())
}

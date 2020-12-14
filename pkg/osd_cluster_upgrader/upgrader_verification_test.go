package osd_cluster_upgrader

import (
	"fmt"
	"strings"

	ac "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks"
	acMocks "github.com/openshift/managed-upgrade-operator/pkg/availabilitychecks/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/clusterversion"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterUpgrader verification and health tests", func() {

	var (
		logger                   logr.Logger
		upgradeConfigName        types.NamespacedName
		upgradeConfig            *upgradev1alpha1.UpgradeConfig
		mockKubeClient           *mocks.MockClient
		mockCtrl                 *gomock.Controller
		mockMaintClient          *mockMaintenance.MockMaintenance
		mockScaler               *mockScaler.MockScaler
		mockMetricsClient        *mockMetrics.MockMetrics
		mockMachinery            *mockMachinery.MockMachinery
		mockCVClient             *cvMocks.MockClusterVersion
		mockDrainStrategyBuilder *mockDrain.MockNodeDrainStrategyBuilder
		mockEMClient             *emMocks.MockEventManager
		mockAC                   *acMocks.MockAvailabilityChecker
		config                   *osdUpgradeConfig
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockMaintClient = mockMaintenance.NewMockMaintenance(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		mockDrainStrategyBuilder = mockDrain.NewMockNodeDrainStrategyBuilder(mockCtrl)
		mockEMClient = emMocks.NewMockEventManager(mockCtrl)
		mockAC = acMocks.NewMockAvailabilityChecker(mockCtrl)

		logger = logf.Log.WithName("cluster upgrader test logger")
		stepCounter = make(map[upgradev1alpha1.UpgradeConditionType]int)
		config = &osdUpgradeConfig{
			Maintenance: maintenanceConfig{
				ControlPlaneTime: 90,
			},
			Scale: scaleConfig{
				TimeOut: 30,
			},
			HealthCheck: healthCheck{
				IgnoredCriticals: []string{"alert1", "alert2"},
			},
			NodeDrain: drain.NodeDrain{
				ExpectedNodeDrainTime: 8,
			},
			Verification: verification{
				IgnoredNamespaces:        []string{"kube-test1"},
				NamespacePrefixesToCheck: []string{"openshift", "default"},
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When performing post-upgrade verification", func() {
		var replicaSetList *appsv1.ReplicaSetList
		var dsList *appsv1.DaemonSetList

		Context("When any core replicasets are not satisfied", func() {
			It("Fails cluster verification", func() {
				replicaSetList = &appsv1.ReplicaSetList{
					Items: []appsv1.ReplicaSet{
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "openshift-logging"},
							Status:     appsv1.ReplicaSetStatus{Replicas: 3, ReadyReplicas: 2},
						},
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "kube-api-server"},
							Status:     appsv1.ReplicaSetStatus{Replicas: 3, ReadyReplicas: 3},
						},
					},
				}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationFailed(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
		Context("When any core daemonsets are not satisfied", func() {
			It("Fails cluster verification", func() {
				replicaSetList = &appsv1.ReplicaSetList{}
				dsList = &appsv1.DaemonSetList{Items: []appsv1.DaemonSet{
					{
						ObjectMeta: v1.ObjectMeta{Namespace: "openshift-logging"},
						Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 2},
					},
					{
						ObjectMeta: v1.ObjectMeta{Namespace: "kube-api-server"},
						Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
					},
				}}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationFailed(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
		Context("When non-core replicasets are not satisfied", func() {
			It("Ignores them", func() {
				replicaSetList = &appsv1.ReplicaSetList{
					Items: []appsv1.ReplicaSet{
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "dummy-namespace"},
							Status:     appsv1.ReplicaSetStatus{Replicas: 3, ReadyReplicas: 2},
						},
					},
				}
				dsList = &appsv1.DaemonSetList{}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().IsAlertFiring(gomock.Any(), gomock.Any(), gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When non-core daemonsets are not satisfied", func() {
			It("Ignores them", func() {
				replicaSetList = &appsv1.ReplicaSetList{}
				dsList = &appsv1.DaemonSetList{Items: []appsv1.DaemonSet{
					{
						ObjectMeta: v1.ObjectMeta{Namespace: "dummy-namespace"},
						Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 2},
					},
				}}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().IsAlertFiring(gomock.Any(), gomock.Any(), gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When core daemonsets and replicasets are satisfied", func() {
			It("Passes cluster verification", func() {
				replicaSetList = &appsv1.ReplicaSetList{
					Items: []appsv1.ReplicaSet{
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "kube-api-server"},
							Status:     appsv1.ReplicaSetStatus{Replicas: 3, ReadyReplicas: 3},
						},
					},
				}
				dsList = &appsv1.DaemonSetList{Items: []appsv1.DaemonSet{
					{
						ObjectMeta: v1.ObjectMeta{Namespace: "default"},
						Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
					},
				}}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().IsAlertFiring(gomock.Any(), gomock.Any(), gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When post-verification alerts are still firing", func() {
			It("Does not pass cluster verification", func() {
				replicaSetList = &appsv1.ReplicaSetList{
					Items: []appsv1.ReplicaSet{
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "kube-api-server"},
							Status:     appsv1.ReplicaSetStatus{Replicas: 3, ReadyReplicas: 3},
						},
					},
				}
				dsList = &appsv1.DaemonSetList{Items: []appsv1.DaemonSet{
					{
						ObjectMeta: v1.ObjectMeta{Namespace: "default"},
						Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
					},
				}}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().IsAlertFiring(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationFailed(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
		Context("When core replicasets are not satisfied but ignored", func() {
			It("Pass cluster verification", func() {
				replicaSetList = &appsv1.ReplicaSetList{
					Items: []appsv1.ReplicaSet{
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "kube-testns"},
							Status:     appsv1.ReplicaSetStatus{Replicas: 3, ReadyReplicas: 1},
						},
					},
				}
				dsList = &appsv1.DaemonSetList{}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().IsAlertFiring(gomock.Any(), gomock.Any(), gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When core daemonsets are not satisfied but ignored", func() {
			It("Pass cluster verification", func() {
				replicaSetList = &appsv1.ReplicaSetList{}
				dsList = &appsv1.DaemonSetList{
					Items: []appsv1.DaemonSet{
						{
							ObjectMeta: v1.ObjectMeta{Namespace: "kube-testns"},
							Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 1},
						},
					},
				}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList),
					mockMetricsClient.EXPECT().IsAlertFiring(gomock.Any(), gomock.Any(), gomock.Any()),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
	})

	Context("When the cluster healthy", func() {
		Context("When no critical alerts are firing", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})
			It("will have ignored some critical alerts", func() {
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil)
				mockMetricsClient.EXPECT().Query(gomock.Any()).DoAndReturn(
					func(query string) (*metrics.AlertResponse, error) {
						Expect(strings.Contains(query, `alertname!="`+config.HealthCheck.IgnoredCriticals[0]+`"`)).To(BeTrue())
						Expect(strings.Contains(query, `alertname!="`+config.HealthCheck.IgnoredCriticals[1]+`"`)).To(BeTrue())
						return &metrics.AlertResponse{}, nil
					})
				mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil)
				mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name)
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(result).To(BeTrue())
			})
			It("will satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name),
				)
				// Pre-upgrade
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When no operators are degraded", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})

			It("will satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name),
				)
				// Pre-upgrade
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
	})

	Context("When the cluster is unhealthy", func() {
		Context("When critical alerts are firing", func() {
			var alertsResponse *metrics.AlertResponse
			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{
					Data: metrics.AlertData{
						Result: []metrics.AlertResult{
							{Metric: make(map[string]string), Value: nil},
							{Metric: make(map[string]string), Value: nil},
						},
					},
				}
			})
			It("will not satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name),
				)
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will not satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When operators are degraded", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})
			It("will not satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{"ClusterOperator"}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name),
				)
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will not satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{"ClusterOperator"}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("When Prometheus can't be queried successfully", func() {
		var fakeError = fmt.Errorf("fake MetricsClient query error")
		BeforeEach(func() {
			mockMetricsClient.EXPECT().Query(gomock.Any()).Return(nil, fakeError)
		})
		It("will abort a cluster health check with the error", func() {
			result, err := performClusterHealthCheck(mockKubeClient, mockMetricsClient, mockCVClient, config, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unable to query critical alerts"))
			Expect(result).To(BeFalse())
		})
		It("will abort Pre-Upgrade check", func() {
			mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil)
			mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name)
			result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unable to query critical alerts"))
			Expect(result).To(BeFalse())
		})
		It("will abort Post-Upgrade check", func() {
			mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name)
			result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockDrainStrategyBuilder, mockMetricsClient, mockMaintClient, mockCVClient, mockEMClient, upgradeConfig, mockMachinery, []ac.AvailabilityChecker{mockAC}, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unable to query critical alerts"))
			Expect(result).To(BeFalse())
		})
	})
})

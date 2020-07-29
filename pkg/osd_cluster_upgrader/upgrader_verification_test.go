package osd_cluster_upgrader

import (
	"fmt"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
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
		logger                 logr.Logger
		upgradeConfigName      types.NamespacedName
		upgradeConfig          *upgradev1alpha1.UpgradeConfig
		mockKubeClient         *mocks.MockClient
		mockCtrl               *gomock.Controller
		mockMaintClient        *mockMaintenance.MockMaintenance
		mockScaler             *mockScaler.MockScaler
		mockMaintenanceBuilder *mockMaintenance.MockMaintenanceBuilder
		mockMetricsClient      *mockMetrics.MockMetrics
		config                 *osdUpgradeConfig
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
		mockMaintenanceBuilder = mockMaintenance.NewMockMaintenanceBuilder(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)

		logger = logf.Log.WithName("cluster upgrader test logger")
		stepCounter = make(map[upgradev1alpha1.UpgradeConditionType]int)
		config = &osdUpgradeConfig{
			Maintenance: maintenanceConfig{
				WorkerNodeTime:   8,
				ControlPlaneTime: 90,
			},
			Scale: scaleConfig{
				TimeOut: 30,
			},
		}
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
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationFailed(upgradeConfig.Name).Times(1),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
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
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationFailed(upgradeConfig.Name).Times(1),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
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
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name).Times(1),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
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
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name).Times(1),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
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
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *replicaSetList).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *dsList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterVerificationSucceeded(upgradeConfig.Name).Times(1),
				)
				result, err := PostUpgradeVerification(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
	})

	Context("When the cluster healthy", func() {
		Context("When no critical alerts are firing", func() {
			var alertsResponse *metrics.AlertResponse
			var operatorList *configv1.ClusterOperatorList

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
				operatorList = &configv1.ClusterOperatorList{}
			})
			It("will satisfy a pre-Upgrade health check", func() {
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name).Times(1),
				)
				// Pre-upgrade
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name).Times(1),
					mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any()).Times(1),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When no operators are degraded", func() {
			var alertsResponse *metrics.AlertResponse
			var operatorList *configv1.ClusterOperatorList

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
				operatorList = &configv1.ClusterOperatorList{
					Items: []configv1.ClusterOperator{
						{
							ObjectMeta: v1.ObjectMeta{
								Name: "operator1",
							},
							Status: configv1.ClusterOperatorStatus{
								Conditions: []configv1.ClusterOperatorStatusCondition{
									{Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue},
								},
							},
						},
						{
							ObjectMeta: v1.ObjectMeta{
								Name: "operator2",
							},
							Status: configv1.ClusterOperatorStatus{
								Conditions: []configv1.ClusterOperatorStatusCondition{
									{Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse},
								},
							},
						},
					},
				}
			})

			It("will satisfy a pre-Upgrade health check", func() {
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name).Times(1),
				)
				// Pre-upgrade
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckSucceeded(upgradeConfig.Name).Times(1),
					mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any()).Times(1),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
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
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name).Times(1),
				)
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will not satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name).Times(1),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When operators are degraded", func() {
			var alertsResponse *metrics.AlertResponse
			var operatorList *configv1.ClusterOperatorList

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
				operatorList = &configv1.ClusterOperatorList{
					Items: []configv1.ClusterOperator{
						{
							ObjectMeta: v1.ObjectMeta{Name: "I'm a broken operator"},
							Spec:       configv1.ClusterOperatorSpec{},
							Status: configv1.ClusterOperatorStatus{
								Conditions: []configv1.ClusterOperatorStatusCondition{
									{Type: configv1.OperatorDegraded, Status: configv1.ConditionTrue},
								},
							},
						},
						{
							ObjectMeta: v1.ObjectMeta{
								Name: "I'm an unavailable operator",
							},
							Spec: configv1.ClusterOperatorSpec{},
							Status: configv1.ClusterOperatorStatus{
								Conditions: []configv1.ClusterOperatorStatusCondition{
									{Type: configv1.OperatorAvailable, Status: configv1.ConditionFalse},
								},
							},
						},
					},
				}
			})
			It("will not satisfy a pre-Upgrade health check", func() {
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name).Times(1),
				)
				result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will not satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(alertsResponse, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name).Times(1),
				)
				result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("When Prometheus can't be queried successfully", func() {
		var fakeError = fmt.Errorf("fake MetricsClient query error")
		BeforeEach(func() {
			mockMetricsClient.EXPECT().Query(gomock.Any()).Times(1).Return(nil, fakeError)
		})
		It("will abort a cluster health check with the error", func() {
			result, err := performClusterHealthCheck(mockKubeClient, mockMetricsClient, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unable to query critical alerts"))
			Expect(result).To(BeFalse())
		})
		It("will abort Pre-Upgrade check", func() {
			expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
			mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name).Times(1)
			result, err := PreClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unable to query critical alerts"))
			Expect(result).To(BeFalse())
		})
		It("will abort Post-Upgrade check", func() {
			mockMetricsClient.EXPECT().UpdateMetricClusterCheckFailed(upgradeConfig.Name).Times(1)
			result, err := PostClusterHealthCheck(mockKubeClient, config, mockScaler, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unable to query critical alerts"))
			Expect(result).To(BeFalse())
		})
	})
})

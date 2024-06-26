package upgraders

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("HealthCheck Step", func() {
	var (
		logger logr.Logger
		// mocks
		mockKubeClient           *mocks.MockClient
		mockCtrl                 *gomock.Controller
		mockMaintClient          *mockMaintenance.MockMaintenance
		mockScalerClient         *mockScaler.MockScaler
		mockMachineryClient      *mockMachinery.MockMachinery
		mockMetricsClient        *mockMetrics.MockMetrics
		mockCVClient             *cvMocks.MockClusterVersion
		mockDrainStrategyBuilder *mockDrain.MockNodeDrainStrategyBuilder
		mockEMClient             *emMocks.MockEventManager

		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig

		// upgrader to be used during tests
		config   *upgraderConfig
		upgrader *clusterUpgrader
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
		mockScalerClient = mockScaler.NewMockScaler(mockCtrl)
		mockMachineryClient = mockMachinery.NewMockMachinery(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		mockDrainStrategyBuilder = mockDrain.NewMockNodeDrainStrategyBuilder(mockCtrl)
		mockEMClient = emMocks.NewMockEventManager(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		config = buildTestUpgraderConfig(90, 30, 8, 120, 30)
		config.HealthCheck = healthCheck{
			IgnoredCriticals:  []string{"alert1", "alert2"},
			IgnoredNamespaces: []string{"ns1"},
		}
		upgrader = &clusterUpgrader{
			client:               mockKubeClient,
			metrics:              mockMetricsClient,
			cvClient:             mockCVClient,
			notifier:             mockEMClient,
			config:               config,
			scaler:               mockScalerClient,
			drainstrategyBuilder: mockDrainStrategyBuilder,
			maintenance:          mockMaintClient,
			machinery:            mockMachineryClient,
			upgradeConfig:        upgradeConfig,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When the cluster healthy", func() {
		nodes := &corev1.NodeList{
			TypeMeta: metav1.TypeMeta{},
			ListMeta: metav1.ListMeta{},
			Items: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
				},
			},
		}
		var cordonAddedTime *metav1.Time
		Context("When no critical alerts are firing", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
				upgradeConfig.Spec.CapacityReservation = true
			})
			It("will satisfy a pre-upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(gomock.Any(), logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: cordonAddedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will have ignored some critical alerts", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).DoAndReturn(
						func(query string) (*metrics.AlertResponse, error) {
							Expect(strings.Contains(query, `alertname!="`+config.HealthCheck.IgnoredCriticals[0]+`"`)).To(BeTrue())
							Expect(strings.Contains(query, `alertname!="`+config.HealthCheck.IgnoredCriticals[1]+`"`)).To(BeTrue())
							return &metrics.AlertResponse{}, nil
						}),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: cordonAddedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(result).To(BeTrue())
			})
			It("will have ignored alerts in specified namespaces", func() {
				mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil)
				mockMetricsClient.EXPECT().Query(gomock.Any()).DoAndReturn(
					func(query string) (*metrics.AlertResponse, error) {
						Expect(strings.Contains(query, `namespace!="`+config.HealthCheck.IgnoredNamespaces[0]+`"`)).To(BeTrue())
						return &metrics.AlertResponse{}, nil
					})
				mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil)
				mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil)
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes)
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: cordonAddedTime})
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes)
				mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false)
				mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false)
				mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false)
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any())
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(Not(HaveOccurred()))
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PostUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When no operators are degraded", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
				upgradeConfig.Spec.CapacityReservation = true
			})

			It("will satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: cordonAddedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PostUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When no node is manually cordoned", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})

			It("will satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: cordonAddedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PostUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When node is cordoned by upgrade", func() {
			var alertsResponse *metrics.AlertResponse

			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})

			It("will satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: cordonAddedTime}),
					mockMachineryClient.EXPECT().IsNodeUpgrading(gomock.Any()).Return(true),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
			It("will satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckSucceeded(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PostUpgradeHealthCheck(context.TODO(), logger)
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
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will not satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PostUpgradeHealthCheck(context.TODO(), logger)
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
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will not satisfy a post-upgrade health check", func() {
				gomock.InOrder(
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{"ClusterOperator"}}, nil),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
				)
				result, err := upgrader.PostUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When node is cordoned manually", func() {
			var alertsResponse *metrics.AlertResponse
			nodes := &corev1.NodeList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
					},
				},
			}
			var cordonAddedTime *metav1.Time
			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})
			It("will not satisfy a pre-Upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: cordonAddedTime}),
					mockMachineryClient.EXPECT().IsNodeUpgrading(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When node unschedulable taint check failed", func() {
			var alertsResponse *metrics.AlertResponse
			var addedTime *metav1.Time
			JustBeforeEach(func() {
				alertsResponse = &metrics.AlertResponse{}
			})
			It("Memory pressure taint will not satisfy a pre-Upgrade health check", func() {
				nodes := &corev1.NodeList{
					TypeMeta: metav1.TypeMeta{},
					ListMeta: metav1.ListMeta{},
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodeMemoryPressure,
										TimeAdded: addedTime,
									},
								},
							},
						},
					},
				}
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: addedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(true),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			It("Disk pressure taint will not satisfy a pre-Upgrade health check", func() {
				nodes := &corev1.NodeList{
					TypeMeta: metav1.TypeMeta{},
					ListMeta: metav1.ListMeta{},
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodeDiskPressure,
										TimeAdded: addedTime,
									},
								},
							},
						},
					},
				}
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: addedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(true),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			It("PID pressure taint will not satisfy a pre-Upgrade health check", func() {
				nodes := &corev1.NodeList{
					TypeMeta: metav1.TypeMeta{},
					ListMeta: metav1.ListMeta{},
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodePIDPressure,
										TimeAdded: addedTime,
									},
								},
							},
						},
					},
				}
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: addedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(true),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			It("There are 2 pressure taints which will not satisfy a pre-Upgrade health check", func() {
				nodes := &corev1.NodeList{
					TypeMeta: metav1.TypeMeta{},
					ListMeta: metav1.ListMeta{},
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodeMemoryPressure,
										TimeAdded: addedTime,
									},
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodePIDPressure,
										TimeAdded: addedTime,
									},
								},
							},
						},
					},
				}
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: addedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(true),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(true),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})

			It("There are all 3 pressure taints which will not satisfy a pre-Upgrade health check", func() {
				nodes := &corev1.NodeList{
					TypeMeta: metav1.TypeMeta{},
					ListMeta: metav1.ListMeta{},
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "memPressureNode"},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodeMemoryPressure,
										TimeAdded: addedTime,
									},
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{Name: "diskAndPIDPressureNode"},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodeDiskPressure,
										TimeAdded: addedTime,
									},
									{
										Effect:    corev1.TaintEffectNoSchedule,
										Key:       corev1.TaintNodePIDPressure,
										TimeAdded: addedTime,
									},
								},
							},
						},
					},
				}
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
					mockMetricsClient.EXPECT().Query(gomock.Any()).Return(alertsResponse, nil),
					mockCVClient.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{Degraded: []string{}}, nil),
					mockScalerClient.EXPECT().CanScale(mockKubeClient, logger).Return(true, nil),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: addedTime}),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: addedTime}),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(true),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasMemoryPressure(gomock.Any()).Return(false),
					mockMachineryClient.EXPECT().HasDiskPressure(gomock.Any()).Return(true),
					mockMachineryClient.EXPECT().HasPidPressure(gomock.Any()).Return(true),
					mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
					mockEMClient.EXPECT().Notify(gomock.Any()).Return(nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When the cluster's upgrade process has commenced", func() {
			It("will not re-perform a pre-upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("When the upgrader can't tell if the cluster's upgrade has commenced", func() {
			var fakeError = fmt.Errorf("fake upgradeCommenced error")
			It("will abort the pre-upgrade health check", func() {
				gomock.InOrder(
					mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, fakeError),
				)
				result, err := upgrader.PreUpgradeHealthCheck(context.TODO(), logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
	})
})

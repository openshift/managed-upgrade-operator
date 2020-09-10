package nodekeeper

import (
	"os"
	"time"

	"github.com/golang/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("NodeKeeperController", func() {
	var (
		reconciler               *ReconcileNodeKeeper
		mockCtrl                 *gomock.Controller
		mockKubeClient           *mocks.MockClient
		mockConfigManagerBuilder *configMocks.MockConfigManagerBuilder
		mockConfigManager        *configMocks.MockConfigManager
		mockMachineryClient      *mockMachinery.MockMachinery
		mockMetricsBuilder       *mockMetrics.MockMetricsBuilder
		mockMetricsClient        *mockMetrics.MockMetrics
		testNodeName             types.NamespacedName
		upgradeConfigName        types.NamespacedName
		upgradeConfigList        upgradev1alpha1.UpgradeConfigList
		config                   nodeKeeperConfig
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockConfigManagerBuilder = configMocks.NewMockConfigManagerBuilder(mockCtrl)
		mockConfigManager = configMocks.NewMockConfigManager(mockCtrl)
		mockMachineryClient = mockMachinery.NewMockMachinery(mockCtrl)
		mockMetricsBuilder = mockMetrics.NewMockMetricsBuilder(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		testNodeName = types.NamespacedName{
			Name: "test-node-1",
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	JustBeforeEach(func() {
		reconciler = &ReconcileNodeKeeper{
			mockKubeClient,
			mockConfigManagerBuilder,
			mockMachineryClient,
			mockMetricsBuilder,
			runtime.NewScheme(),
		}
	})

	Context("Reconcile", func() {
		BeforeEach(func() {
			ns := "openshift-managed-upgrade-operator"
			upgradeConfigName = types.NamespacedName{
				Name:      "test-upgradeconfig",
				Namespace: ns,
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", ns)
		})
		Context("When to check nodes", func() {
			It("should not check node if not in upgrade phase", func() {
				uc := *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhasePending).GetUpgradeConfig()
				upgradeConfigList = upgradev1alpha1.UpgradeConfigList{
					Items: []upgradev1alpha1.UpgradeConfig{uc},
				}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, upgradeConfigList).Return(nil),
					mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true}, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), testNodeName, gomock.Any()).Times(0),
				)
				_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: testNodeName})
				Expect(err).NotTo(HaveOccurred())
			})
			It("should not check node if machines are not upgrading", func() {
				uc := *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseUpgrading).GetUpgradeConfig()
				upgradeConfigList = upgradev1alpha1.UpgradeConfigList{
					Items: []upgradev1alpha1.UpgradeConfig{uc},
				}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, upgradeConfigList).Return(nil),
					mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: false}, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), testNodeName, gomock.Any()).Times(0),
				)
				_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: testNodeName})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Alerting for node drain problems", func() {
			BeforeEach(func() {
				uc := *testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).WithPhase(upgradev1alpha1.UpgradePhaseUpgrading).GetUpgradeConfig()
				upgradeConfigList = upgradev1alpha1.UpgradeConfigList{
					Items: []upgradev1alpha1.UpgradeConfig{uc},
				}
				config = nodeKeeperConfig{
					NodeDrain: machinery.NodeDrain{
						Timeout: 5,
					},
				}
			})
			It("should alert when a node drain takes too long", func() {
				testNode := corev1.Node{
					Spec: corev1.NodeSpec{
						Unschedulable: true,
						Taints: []corev1.Taint{
							{Effect: corev1.TaintEffectNoSchedule,
								TimeAdded: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
							},
						},
					},
				}

				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, upgradeConfigList).Return(nil),
					mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true}, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), testNodeName, gomock.Any()).SetArg(2, testNode).Return(nil),
					mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
					mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, config),
					mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetricsClient, nil),
					mockMetricsClient.EXPECT().UpdateMetricNodeDrainFailed(gomock.Any()).Times(1),
					mockMetricsClient.EXPECT().ResetMetricNodeDrainFailed(gomock.Any()).Times(0),
				)
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: testNodeName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
			It("should reset any alerts once node is not draining", func() {
				testNode := corev1.Node{
					Spec: corev1.NodeSpec{
						Unschedulable: false,
					},
				}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, upgradeConfigList).Return(nil),
					mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true}, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), testNodeName, gomock.Any()).SetArg(2, testNode).Return(nil),
					mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
					mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, config),
					mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetricsClient, nil),
					mockMetricsClient.EXPECT().UpdateMetricNodeDrainFailed(gomock.Any()).Times(0),
					mockMetricsClient.EXPECT().ResetMetricNodeDrainFailed(gomock.Any()).Times(1),
				)
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: testNodeName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})
	})
})

package nodekeeper

import (
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	mockDrain "github.com/openshift/managed-upgrade-operator/pkg/drain/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
		mockDrainStrategyBuilder *mockDrain.MockNodeDrainStrategyBuilder
		mockDrainStrategy        *mockDrain.MockNodeDrainStrategy
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
		mockDrainStrategyBuilder = mockDrain.NewMockNodeDrainStrategyBuilder(mockCtrl)
		mockDrainStrategy = mockDrain.NewMockNodeDrainStrategy(mockCtrl)
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
			mockDrainStrategyBuilder,
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
					NodeDrain: drain.NodeDrain{
						Timeout:               5,
						ExpectedNodeDrainTime: 8,
					},
				}
			})
			It("should alert when a node drain takes too long", func() {
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, upgradeConfigList).Return(nil),
					mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true}, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), testNodeName, gomock.Any()).Times(1),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: &metav1.Time{Time: time.Now().Add(-10 * time.Minute)}}),
					mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetricsClient, nil),
					mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
					mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, config),
					mockDrainStrategyBuilder.EXPECT().NewNodeDrainStrategy(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockDrainStrategy, nil),
					mockDrainStrategy.EXPECT().Execute(gomock.Any()).Return([]*drain.DrainStrategyResult{}, nil),
					mockDrainStrategy.EXPECT().HasFailed(gomock.Any()).Return(true, nil),
					mockMetricsClient.EXPECT().UpdateMetricNodeDrainFailed(gomock.Any()).Times(1),
					mockMetricsClient.EXPECT().ResetMetricNodeDrainFailed(gomock.Any()).Times(0),
				)
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: testNodeName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(Not(BeNil()))
			})
			It("should reset any alerts once node is not cordoned", func() {
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, upgradeConfigList).Return(nil),
					mockMachineryClient.EXPECT().IsUpgrading(gomock.Any(), "worker").Return(&machinery.UpgradingResult{IsUpgrading: true}, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), testNodeName, gomock.Any()).Times(1),
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false}),
					mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetricsClient, nil),
					mockMetricsClient.EXPECT().ResetMetricNodeDrainFailed(gomock.Any()).Times(1),
					mockMetricsClient.EXPECT().UpdateMetricNodeDrainFailed(gomock.Any()).Times(0),
				)
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: testNodeName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})
	})
})

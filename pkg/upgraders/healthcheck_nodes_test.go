package upgraders

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	machineryMocks "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("HealthCheck Manually Cordoned node", func() {
	var (
		logger              logr.Logger
		mockCtrl            *gomock.Controller
		mockKubeClient      *mocks.MockClient
		mockMetricsClient   *mockMetrics.MockMetrics
		mockMachineryClient *machineryMocks.MockMachinery

		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig

		// upgrader to be used during tests
		config *upgraderConfig
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockMachineryClient = machineryMocks.NewMockMachinery(mockCtrl)
		logger = logf.Log.WithName("cluster upgrader test logger")
		config = buildTestUpgraderConfig(90, 30, 8, 120, 30)
		config.HealthCheck = healthCheck{
			IgnoredCriticals:  []string{"alert1", "alert2"},
			IgnoredNamespaces: []string{"ns1"},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When no node is manually cordoned", func() {
		It("Prehealth check will pass", func() {
			var cordonAddedTime *metav1.Time
			nodes := &corev1.NodeList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false, AddedAt: cordonAddedTime}),
			)
			result, err := ManuallyCordonedNodes(mockMetricsClient, mockMachineryClient, mockKubeClient, upgradeConfig, logger)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).Should(BeTrue())
		})
	})

	Context("When there are cordoned nodes because of the upgrade", func() {
		It("Prehealth check will pass", func() {
			var cordonAddedTime *metav1.Time
			nodes := &corev1.NodeList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: cordonAddedTime}),
				mockMachineryClient.EXPECT().IsNodeUpgrading(gomock.Any()).Return(true),
			)
			result, err := ManuallyCordonedNodes(mockMetricsClient, mockMachineryClient, mockKubeClient, upgradeConfig, logger)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(result).Should(BeTrue())
		})
	})

	Context("When get all worker nodes failed", func() {
		It("Prehealth check will fail", func() {
			mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("Fake cannot fetch all worker nodes"))
			mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any())
			result, err := ManuallyCordonedNodes(mockMetricsClient, mockMachineryClient, mockKubeClient, upgradeConfig, logger)
			Expect(err).Should(HaveOccurred())
			Expect(result).Should(BeFalse())
		})
	})

	Context("When there are manually cordoned nodes ", func() {
		It("Prehealth check will fail", func() {
			var cordonAddedTime *metav1.Time
			nodes := &corev1.NodeList{
				TypeMeta: metav1.TypeMeta{},
				ListMeta: metav1.ListMeta{},
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "testNode"},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).SetArg(1, *nodes),
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: cordonAddedTime}),
				mockMachineryClient.EXPECT().IsNodeUpgrading(gomock.Any()).Return(false),
				mockMetricsClient.EXPECT().UpdateMetricHealthcheckFailed(upgradeConfig.Name, gomock.Any()),
			)
			result, err := ManuallyCordonedNodes(mockMetricsClient, mockMachineryClient, mockKubeClient, upgradeConfig, logger)
			Expect(err).Should(HaveOccurred())
			Expect(result).Should(BeFalse())
		})
	})
})

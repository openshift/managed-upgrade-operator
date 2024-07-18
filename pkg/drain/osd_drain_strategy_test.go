package drain

import (
	"fmt"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
	mockMachinery "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	mockNotifier "github.com/openshift/managed-upgrade-operator/pkg/notifier/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/pod"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_OPERATOR_NAMESPACE = "test-namespace"
	TEST_UPGRADECONFIG_CR   = "managed-upgrade-config"
	TEST_UPGRADE_VERSION    = "4.14.14"
	TEST_UPGRADE_CHANNEL    = "stable-4.14"
	TEST_UPGRADE_TIME       = "2024-06-20T00:00:00Z"
	TEST_UPGRADE_PDB_TIME   = 60
	TEST_UPGRADE_TYPE       = "OSD"
)

var _ = Describe("OSD Drain Strategy", func() {

	var (
		logger              logr.Logger
		mockCtrl            *gomock.Controller
		mockKubeClient      *mocks.MockClient
		mockMachineryClient *mockMachinery.MockMachinery
		osdDrain            NodeDrainStrategy
		mockTimedDrainOne   *MockTimedDrainStrategy
		mockStrategyOne     *MockDrainStrategy
		mockTimedDrainTwo   *MockTimedDrainStrategy
		mockStrategyTwo     *MockDrainStrategy
		nodeDrainConfig     *NodeDrain
		mockUpgradeConfig   *upgradev1alpha1.UpgradeConfig
		mockNotifierClient  *mockNotifier.MockNotifier
		mockMetricsClient   *mockMetrics.MockMetrics
	)

	Context("Node drain Time Based Strategy execution", func() {
		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			mockKubeClient = mocks.NewMockClient(mockCtrl)
			mockMachineryClient = mockMachinery.NewMockMachinery(mockCtrl)
			mockTimedDrainOne = NewMockTimedDrainStrategy(mockCtrl)
			mockStrategyOne = NewMockDrainStrategy(mockCtrl)
			mockTimedDrainTwo = NewMockTimedDrainStrategy(mockCtrl)
			mockStrategyTwo = NewMockDrainStrategy(mockCtrl)
			logger = logf.Log.WithName("drain strategy test logger")
			mockNotifierClient = mockNotifier.NewMockNotifier(mockCtrl)
			mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
			mockUpgradeConfig = &upgradev1alpha1.UpgradeConfig{
				ObjectMeta: metav1.ObjectMeta{
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
			}
		})
		AfterEach(func() {
			mockCtrl.Finish()
		})
		It("should not error if there are no Strategies", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				mockMachineryClient,
				&NodeDrain{},
				[]TimedDrainStrategy{},
				mockUpgradeConfig,
				mockNotifierClient,
				mockMetricsClient,
			}
			fiveMinsAgo := &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
			gomock.InOrder(
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: fiveMinsAgo}),
			)
			result, err := osdDrain.Execute(&corev1.Node{}, logger)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(0))
		})
		It("should execute a Time Based Drain Strategy after the assigned wait duration", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				mockMachineryClient,
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne},
				mockUpgradeConfig,
				mockNotifierClient,
				mockMetricsClient,
			}
			fortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			gomock.InOrder(
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: fortyFiveMinsAgo}),
				mockTimedDrainOne.EXPECT().GetName().Return("test strategy"),
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*30).Times(2),
				mockTimedDrainOne.EXPECT().GetStrategy().Return(mockStrategyOne),
				mockStrategyOne.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(1).Return(&DrainStrategyResult{Message: "", HasExecuted: true}, nil),
			)
			result, err := osdDrain.Execute(&corev1.Node{}, logger)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(1))
		})
		It("should not execute a Time Based Drain Strategy before the assigned duration", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				mockMachineryClient,
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne},
				mockUpgradeConfig,
				mockNotifierClient,
				mockMetricsClient,
			}
			fortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			gomock.InOrder(
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: fortyFiveMinsAgo}),
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*60).Times(2),
				mockStrategyOne.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0),
				mockTimedDrainOne.EXPECT().GetDescription().Times(0).Return("Drain one"),
				mockTimedDrainOne.EXPECT().GetName().Return("test strategy"),
			)
			result, err := osdDrain.Execute(&corev1.Node{}, logger)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(0))
		})
		It("should only execute Time Based Drain Strategy at the correct time if multiple strategies exist", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				mockMachineryClient,
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne, mockTimedDrainTwo},
				mockUpgradeConfig,
				mockNotifierClient,
				mockMetricsClient,
			}
			fortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			gomock.InOrder(
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: fortyFiveMinsAgo}),
				mockTimedDrainOne.EXPECT().GetName().Return("test strategy"),
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*30).Times(2),
				mockTimedDrainOne.EXPECT().GetStrategy().Return(mockStrategyOne),
				mockStrategyOne.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(1).Return(&DrainStrategyResult{Message: "", HasExecuted: true}, nil),
				mockTimedDrainTwo.EXPECT().GetName().Return("test strategy"),
				mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(time.Minute*60).Times(2),
				mockStrategyTwo.EXPECT().Execute(gomock.Any(), gomock.Any()).Times(0),
			)
			result, err := osdDrain.Execute(&corev1.Node{}, logger)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(1))
		})
		It("should return an error if the node drain time is nil", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				mockMachineryClient,
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne},
				mockUpgradeConfig,
				mockNotifierClient,
				mockMetricsClient,
			}
			gomock.InOrder(
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Times(1).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: nil}),
			)
			_, err := osdDrain.Execute(&corev1.Node{}, logger)
			Expect(err).NotTo(BeNil())
		})
		It("should return an error if the node drain strategy fails", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				mockMachineryClient,
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne},
				mockUpgradeConfig,
				mockNotifierClient,
				mockMetricsClient,
			}
			fortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			gomock.InOrder(
				mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: fortyFiveMinsAgo}),
				mockTimedDrainOne.EXPECT().GetName().Return("test strategy"),
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*30).Times(2),
				mockTimedDrainOne.EXPECT().GetStrategy().Return(mockStrategyOne),
				mockStrategyOne.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("fake error")),
			)
			_, err := osdDrain.Execute(&corev1.Node{}, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Node Drain Strategies failures", func() {
		Context("When there are no strategies", func() {
			BeforeEach(func() {
				mockCtrl = gomock.NewController(GinkgoT())
				mockKubeClient = mocks.NewMockClient(mockCtrl)
				nodeDrainConfig = &NodeDrain{
					Timeout: 45,
				}
				osdDrain = &osdDrainStrategy{
					mockKubeClient,
					mockMachineryClient,
					nodeDrainConfig,
					[]TimedDrainStrategy{},
					mockUpgradeConfig,
					mockNotifierClient,
					mockMetricsClient,
				}
			})
			AfterEach(func() {
				mockCtrl.Finish()
			})
			It("should not fail before default timeout wait has elapsed", func() {
				notLongEnough := &metav1.Time{Time: time.Now().Add(nodeDrainConfig.GetTimeOutDuration() / 2)}
				gomock.InOrder(
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: notLongEnough}),
				)
				result, err := osdDrain.HasFailed(&corev1.Node{}, logger)
				Expect(result).To(BeFalse())
				Expect(err).To(BeNil())
			})
			It("should fail after default timeout wait has elapsed", func() {
				tooLongAgo := &metav1.Time{Time: time.Now().Add(-2 * nodeDrainConfig.GetTimeOutDuration())}
				gomock.InOrder(
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: tooLongAgo}),
				)
				result, err := osdDrain.HasFailed(&corev1.Node{}, logger)
				Expect(result).To(BeTrue())
				Expect(err).To(BeNil())
			})
		})

		Context("Node drain Time Based Strategy failure", func() {
			BeforeEach(func() {
				mockCtrl = gomock.NewController(GinkgoT())
				mockKubeClient = mocks.NewMockClient(mockCtrl)
				mockTimedDrainOne = NewMockTimedDrainStrategy(mockCtrl)
				mockTimedDrainTwo = NewMockTimedDrainStrategy(mockCtrl)
				nodeDrainConfig = &NodeDrain{
					ExpectedNodeDrainTime: 8,
					Timeout:               15,
				}
				osdDrain = &osdDrainStrategy{
					mockKubeClient,
					mockMachineryClient,
					nodeDrainConfig,
					[]TimedDrainStrategy{mockTimedDrainTwo, mockTimedDrainOne},
					mockUpgradeConfig,
					mockNotifierClient,
					mockMetricsClient,
				}
			})
			AfterEach(func() {
				mockCtrl.Finish()
			})
			It("should fail after the last strategy has failed + allowed time for drain to occur", func() {
				drainStartedSixtyNineMinsAgo := &metav1.Time{Time: time.Now().Add(-69 * time.Minute)}
				mockOneDuration := time.Minute * 30
				mockTwoDuration := time.Minute * 60
				gomock.InOrder(
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: drainStartedSixtyNineMinsAgo}),
					// Need to use 'Any' as the sort function calls these functions many times
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration).AnyTimes(),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration).AnyTimes(),
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
				)
				result, err := osdDrain.HasFailed(&corev1.Node{}, logger)
				Expect(result).To(BeTrue())
				Expect(err).To(BeNil())
			})
			It("should fail after default timeout wait has elapsed + allowed time for drain to occur if max strategy wait duration is less", func() {
				mockOneDuration := time.Minute * 5
				mockTwoDuration := time.Minute * 10
				thirtyOneMinsAgo := &metav1.Time{Time: time.Now().Add(-16*time.Minute - nodeDrainConfig.GetTimeOutDuration())}
				gomock.InOrder(
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: thirtyOneMinsAgo}),
					// Need to use 'Any' as the sort function calls these functions many times
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration).AnyTimes(),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration).AnyTimes(),
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
				)

				result, _ := osdDrain.HasFailed(&corev1.Node{}, logger)
				Expect(result).To(BeTrue())
			})
			It("should not fail if there are pending strategies", func() {
				mockOneDuration := time.Minute * 10
				mockTwoDuration := time.Minute * 30
				twentyMinsAgo := &metav1.Time{Time: time.Now().Add(-20 * time.Minute)}
				gomock.InOrder(
					mockMachineryClient.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true, AddedAt: twentyMinsAgo}),
					// Need to use 'Any' as the sort function calls these functions many times
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration).AnyTimes(),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration).AnyTimes(),
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
					mockTimedDrainTwo.EXPECT().GetStrategy().Return(mockStrategyOne),
					mockStrategyOne.EXPECT().IsValid(gomock.Any(), gomock.Any()).Return(true, nil),
				)

				result, _ := osdDrain.HasFailed(&corev1.Node{}, logger)
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("Pod Predicates", func() {
		var (
			podList *corev1.PodList
		)

		Context("PDB Pods", func() {
			var (
				pdbPodName  = "test-pdb-pod"
				pdbAppKey   = "app"
				pdbAppValue = "app1"
				pdbList     *policyv1.PodDisruptionBudgetList
			)
			BeforeEach(func() {
				pdbList = &policyv1.PodDisruptionBudgetList{
					Items: []policyv1.PodDisruptionBudget{
						{
							Spec: policyv1.PodDisruptionBudgetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										pdbAppKey: pdbAppValue,
									},
								},
							},
						},
						{
							Spec: policyv1.PodDisruptionBudgetSpec{
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"non-existent-pod-selector": "",
									},
								},
							},
						},
					},
				}
				podList = &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: pdbPodName,
								Labels: map[string]string{
									pdbAppKey:     pdbAppValue,
									"other-label": "label1",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app":         "app2",
									"other-label": "label2",
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app":         "app3",
									"other-label": "label3",
								},
							},
						},
					},
				}
			})
			It("should return pods that have an associated PodDisruptionBudget", func() {
				filteredPods := pod.FilterPods(podList, isPdbPod(pdbList))
				Expect(len(filteredPods.Items)).To(Equal(1))
				Expect(filteredPods.Items[0].Name).To(Equal(pdbPodName))
			})
			It("should return pods that do not have an associated PodDisruptionBudget", func() {
				filteredPods := pod.FilterPods(podList, isNotPdbPod(pdbList))
				Expect(len(filteredPods.Items)).To(Equal(2))
				Expect(filteredPods.Items[0].Name).To(Not(Equal(pdbPodName)))
				Expect(filteredPods.Items[1].Name).To(Not(Equal(pdbPodName)))
			})
		})

		Context("Pods on a Node", func() {
			var (
				podOnNode       = "test-pdb-pod"
				nodeWhichHasPod = "test-node"
				nodePodIsOn     = &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: nodeWhichHasPod,
					},
				}
				nodePodIsNotOn = &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "dummy node",
					},
				}
			)
			BeforeEach(func() {
				podList = &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: podOnNode,
							},
							Spec: corev1.PodSpec{
								NodeName: nodeWhichHasPod,
							},
						},
						{
							Spec: corev1.PodSpec{
								NodeName: podOnNode + "no",
							},
						},
						{
							Spec: corev1.PodSpec{
								NodeName: podOnNode + "also no",
							},
						},
					},
				}
			})
			It("should return pods that are on a specific node", func() {
				filteredPods := pod.FilterPods(podList, isOnNode(nodePodIsOn))
				Expect(len(filteredPods.Items)).To(Equal(1))
				Expect(filteredPods.Items[0].Name).To(Equal(podOnNode))
			})
			It("should not return pods that are on a specific node", func() {
				filteredPods := pod.FilterPods(podList, isOnNode(nodePodIsNotOn))
				Expect(len(filteredPods.Items)).To(Equal(0))
			})
		})

		Context("DaemonSet Pods", func() {
			var (
				daemonsetPodName = "test-pdb-pod"
			)
			BeforeEach(func() {
				podList = &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: daemonsetPodName,
								OwnerReferences: []metav1.OwnerReference{
									{
										Kind: "DaemonSet",
									},
								},
							},
						},
						{},
						{},
					},
				}
			})
			It("should return pods that are part of a DaemonSet", func() {
				filteredPods := pod.FilterPods(podList, isDaemonSet)
				Expect(len(filteredPods.Items)).To(Equal(1))
				Expect(filteredPods.Items[0].Name).To(Equal(daemonsetPodName))
			})
			It("should return pods that are not part of a DaemonSet", func() {
				filteredPods := pod.FilterPods(podList, isNotDaemonSet)
				Expect(len(filteredPods.Items)).To(Equal(2))
				Expect(filteredPods.Items[0].Name).To(Not(Equal(daemonsetPodName)))
				Expect(filteredPods.Items[1].Name).To(Not(Equal(daemonsetPodName)))
			})
		})
		Context("Pod Finalizers", func() {
			It("should return pods that have a finalizer", func() {
				podList = &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Finalizers: []string{"a.finalizer.com"},
							},
						},
					},
				}
				filteredPods := pod.FilterPods(podList, hasFinalizers)
				Expect(len(filteredPods.Items)).To(Equal(1))
			})
			It("should not return pods that have no finalizers", func() {
				podList = &corev1.PodList{
					Items: []corev1.Pod{{}},
				}
				filteredPods := pod.FilterPods(podList, hasFinalizers)
				Expect(len(filteredPods.Items)).To(Equal(0))
			})
		})
		Context("Pods terminating", func() {
			It("should return pods that are terminating", func() {
				podList = &corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								DeletionTimestamp: &metav1.Time{Time: time.Now()},
							},
						},
					},
				}
				filteredPods := pod.FilterPods(podList, isTerminating)
				Expect(len(filteredPods.Items)).To(Equal(1))
			})
			It("should not return pods that are not terminating", func() {
				podList = &corev1.PodList{
					Items: []corev1.Pod{{}},
				}
				filteredPods := pod.FilterPods(podList, isTerminating)
				Expect(len(filteredPods.Items)).To(Equal(0))
			})
		})

	})
})

package cluster_upgrader

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClusterUpgrader node scaling tests", func() {

	var (
		logger            logr.Logger
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		mockKubeClient    *mocks.MockClient
		mockCtrl          *gomock.Controller
		mockMaintClient   *mockMaintenance.MockMaintenance
		mockMetricsClient *mockMetrics.MockMetrics
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

		logger = logf.Log.WithName("cluster upgrader test logger")
		stepCounter = make(map[upgradev1alpha1.UpgradeConditionType]int)
	})

	Context("When the upgrade is scaling out workers", func() {
		var upgradeMachinesets *machineapi.MachineSetList
		var originalMachineSets *machineapi.MachineSetList
		Context("When looking for the upgrade machineset fails", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
				}).Times(1).Return(fakeError)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		Context("When looking for original machinesets fails", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).Times(1).Return(fakeError),
				)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		Context("When no original machineset appears to exist", func() {
			It("Indicates an error", func() {
				originalMachineSets = &machineapi.MachineSetList{}
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
				)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get original machineset"))
				Expect(result).To(BeFalse())
			})
		})

		Context("When we haven't yet started scaling out", func() {
			BeforeEach(func() {
				upgradeMachinesets = &machineapi.MachineSetList{}
				originalMachineSets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      "test-infra",
								Namespace: "openshift-machine-api",
							},
							Spec: machineapi.MachineSetSpec{
								Selector: v1.LabelSelector{
									MatchLabels:      make(map[string]string),
									MatchExpressions: nil,
								},
								Template: machineapi.MachineTemplateSpec{
									ObjectMeta: v1.ObjectMeta{
										Labels: make(map[string]string),
									},
								},
							},
							Status: machineapi.MachineSetStatus{},
						},
					},
				}
			})
			It("will create an upgrade machineset", func() {
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
					mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
						func(ctx context.Context, ms *machineapi.MachineSet) error {
							Expect(ms.Name).To(Equal(originalMachineSets.Items[0].Name + "-upgrade"))
							Expect(ms.Namespace).To(Equal(originalMachineSets.Items[0].Namespace))
							Expect(ms.Labels[LABEL_UPGRADE]).To(Equal("true"))
							Expect(*ms.Spec.Replicas).To(Equal(int32(1)))
							Expect(ms.Spec.Template.Labels[LABEL_UPGRADE]).To(Equal("true"))
							Expect(ms.Spec.Selector.MatchLabels[LABEL_UPGRADE]).To(Equal("true"))
							return nil
						}),
				)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will indicate an error if it cannot create an upgrade machineset", func() {
				fakeError := fmt.Errorf("fake error")
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
					mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).Times(1).Return(fakeError),
				)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())

			})
		})

		Context("When we're waiting for scale-out to finish", func() {
			It("Indicates that scaling has not yet completed", func() {
				upgradeMachinesets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      "test-infra-upgrade",
								Namespace: "openshift-machine-api",
							},
							Status: machineapi.MachineSetStatus{
								Replicas:      1,
								ReadyReplicas: 0,
							},
						},
					},
				}
				originalMachineSets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      "test-infra",
								Namespace: "openshift-machine-api",
							},
						},
					},
				}
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).Times(1),
				)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})

		Context("When scaled nodes are not ready", func() {
			var (
				testMachineName      = "test-machine"
				testMachineNamespace = "openshift-machine-api"
				testMachineSet       = "test-infra"
				upgradeMachines      *machineapi.MachineList
				nodes                *corev1.NodeList
			)
			JustBeforeEach(func() {
				upgradeMachines = &machineapi.MachineList{
					Items: []machineapi.Machine{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      testMachineName,
								Namespace: testMachineNamespace,
								Labels:    map[string]string{LABEL_UPGRADE: "true"},
							},
							Spec:   machineapi.MachineSpec{},
							Status: machineapi.MachineStatus{},
						},
					},
				}
				nodes = &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: v1.ObjectMeta{
								Annotations: map[string]string{
									"machine.openshift.io/machine": "openshift-machine-api/" + testMachineName,
								},
							},
							Status: corev1.NodeStatus{
								Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}},
							},
						},
					},
				}
				upgradeMachinesets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:              testMachineSet + "-upgrade",
								Namespace:         "openshift-machine-api",
								CreationTimestamp: v1.Time{Time: time.Now()},
								Labels:            map[string]string{LABEL_UPGRADE: "true"},
							},
							Status: machineapi.MachineSetStatus{
								Replicas:      1,
								ReadyReplicas: 1,
							},
						},
					},
				}
				originalMachineSets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      testMachineSet,
								Namespace: "openshift-machine-api",
							},
						},
					},
				}
			})
			Context("When a scaled node is not ready within 30 minutes", func() {
				JustBeforeEach(func() {
					upgradeMachinesets.Items[0].ObjectMeta.CreationTimestamp = v1.Time{Time: time.Now().Add(-60 * time.Minute)}
				})
				It("Raises an error", func() {
					expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
					gomock.InOrder(
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
							client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
						}).SetArg(1, *upgradeMachinesets).Times(1),
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
							client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
						}).SetArg(1, *originalMachineSets).Times(1),
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *nodes).Times(1),
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
							client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
						}).SetArg(1, *upgradeMachines).Times(1),
					)
					result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("timeout waiting for node"))
					Expect(result).To(BeFalse())
				})
			})
			Context("When a scaled node is not ready", func() {
				It("Indicates that scaling has not completed", func() {
					expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
					gomock.InOrder(
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
							client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
						}).SetArg(1, *upgradeMachinesets).Times(1),
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
							client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
						}).SetArg(1, *originalMachineSets).Times(1),
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *nodes).Times(1),
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
							client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
						}).SetArg(1, *upgradeMachines).Times(1),
					)
					result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})
		})

		Context("When all scaled nodes are ready", func() {
			It("Indicates scaling has completed", func() {
				testMachineName := "test-machine"
				testMachineNamespace := "openshift-machine-api"
				testMachineSet := "test-infra"
				upgradeMachines := &machineapi.MachineList{
					Items: []machineapi.Machine{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:              testMachineName,
								Namespace:         testMachineNamespace,
								CreationTimestamp: v1.Time{Time: time.Now()},
								Labels:            map[string]string{LABEL_UPGRADE: "true"},
							},
							Spec:   machineapi.MachineSpec{},
							Status: machineapi.MachineStatus{},
						},
					},
				}
				nodes := &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: v1.ObjectMeta{
								Annotations: map[string]string{
									"machine.openshift.io/machine": "openshift-machine-api/" + testMachineName,
								},
							},
							Status: corev1.NodeStatus{
								Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
							},
						},
					},
				}
				upgradeMachinesets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      testMachineSet + "-upgrade",
								Namespace: "openshift-machine-api",
							},
							Status: machineapi.MachineSetStatus{
								Replicas:      1,
								ReadyReplicas: 1,
							},
						},
					},
				}
				originalMachineSets = &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						{
							ObjectMeta: v1.ObjectMeta{
								Name:      testMachineSet,
								Namespace: "openshift-machine-api",
							},
						},
					},
				}
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, *nodes).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachines).Times(1),
				)
				result, err := EnsureExtraUpgradeWorkers(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

	})

	Context("When the upgrade is scaling in workers", func() {
		var upgradeMachinesets *machineapi.MachineSetList
		BeforeEach(func() {
			upgradeMachinesets = &machineapi.MachineSetList{
				Items: []machineapi.MachineSet{
					{ObjectMeta: v1.ObjectMeta{Name: "scaled1"}},
					{ObjectMeta: v1.ObjectMeta{Name: "scaled2"}},
				},
			}
		})
		Context("When the scaled machines can't be fetched", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace("openshift-machine-api"),
					client.MatchingLabels{LABEL_UPGRADE: "true"},
				}).SetArg(1, *upgradeMachinesets).Times(1).Return(fakeError)
				result, err := RemoveExtraScaledNodes(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		Context("When a scaled machine can't be deleted", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace("openshift-machine-api"),
					client.MatchingLabels{LABEL_UPGRADE: "true"},
				}).SetArg(1, *upgradeMachinesets).Times(1)
				// The first delete will cause the whole thing to bail out
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(fakeError)
				result, err := RemoveExtraScaledNodes(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		It("Deletes all scaled machines", func() {
			mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
				client.InNamespace("openshift-machine-api"),
				client.MatchingLabels{LABEL_UPGRADE: "true"},
			}).SetArg(1, *upgradeMachinesets).Times(1)
			// Verify that every specific machine returned to scale down actually does get deleted
			mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, set *machineapi.MachineSet) error {
					found := false
					for _, m := range upgradeMachinesets.Items {
						if set.Name == m.Name {
							found = true
						}
					}
					Expect(found).To(BeTrue())
					return nil
				}).Times(2)
			result, err := RemoveExtraScaledNodes(mockKubeClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})
})

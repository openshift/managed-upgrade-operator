package scaler

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Node scaling tests", func() {

	var (
		logger         logr.Logger
		mockKubeClient *mocks.MockClient
		mockCtrl       *gomock.Controller
		scaler         Scaler
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)

		scaler = &machineSetScaler{}
		logger = logf.Log.WithName("Config validation test logger")
	})

	Context("When the upgrade is scaling out workers", func() {
		var upgradeMachinesets *machineapi.MachineSetList
		var originalMachineSets *machineapi.MachineSetList
		testDuration := 30 * time.Minute
		Context("When looking for the upgrade machineset fails", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
				}).Times(1).Return(fakeError)
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		Context("When looking for original machinesets fails", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).Times(1).Return(fakeError),
				)
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		Context("When no original machineset appears to exist", func() {
			It("Indicates an error", func() {
				originalMachineSets = &machineapi.MachineSetList{}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
				)
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
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
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-infra",
								Namespace: "openshift-machine-api",
							},
							Spec: machineapi.MachineSetSpec{
								Selector: metav1.LabelSelector{
									MatchLabels:      make(map[string]string),
									MatchExpressions: nil,
								},
								Template: machineapi.MachineTemplateSpec{
									ObjectMeta: metav1.ObjectMeta{
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
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
				)
				mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(
					func(ctx context.Context, ms *machineapi.MachineSet) error {
						Expect(ms.Name).To(Equal(originalMachineSets.Items[0].Name + "-upgrade"))
						Expect(ms.Namespace).To(Equal(originalMachineSets.Items[0].Namespace))
						Expect(ms.Labels[LABEL_UPGRADE]).To(Equal("true"))
						Expect(*ms.Spec.Replicas).To(Equal(int32(1)))
						Expect(ms.Spec.Template.Labels[LABEL_UPGRADE]).To(Equal("true"))
						Expect(ms.Spec.Selector.MatchLabels[LABEL_UPGRADE]).To(Equal("true"))
						return nil
					})
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
			It("will indicate an error if it cannot create an upgrade machineset", func() {
				fakeError := fmt.Errorf("fake error")
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
				)
				mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).Times(1).Return(fakeError)
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
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
							ObjectMeta: metav1.ObjectMeta{
								Name:              "test-infra-upgrade",
								Namespace:         "openshift-machine-api",
								CreationTimestamp: metav1.Time{Time: time.Now()},
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
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-infra",
								Namespace: "openshift-machine-api",
							},
						},
					},
				}
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{LABEL_UPGRADE: "true"},
					}).SetArg(1, *upgradeMachinesets).Times(1),
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
						client.InNamespace("openshift-machine-api"), client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
					}).SetArg(1, *originalMachineSets).Times(1),
				)
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).Times(1)
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
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
							ObjectMeta: metav1.ObjectMeta{
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
							ObjectMeta: metav1.ObjectMeta{
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
							ObjectMeta: metav1.ObjectMeta{
								Name:              testMachineSet + "-upgrade",
								Namespace:         "openshift-machine-api",
								CreationTimestamp: metav1.Time{Time: time.Now()},
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
							ObjectMeta: metav1.ObjectMeta{
								Name:      testMachineSet,
								Namespace: "openshift-machine-api",
							},
						},
					},
				}
			})
			Context("When a scaled node is not ready within 30 minutes", func() {
				JustBeforeEach(func() {
					upgradeMachinesets.Items[0].ObjectMeta.CreationTimestamp = metav1.Time{Time: time.Now().Add(-60 * time.Minute)}
				})
				It("Raises an error", func() {
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
					result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
					Expect(err).To(HaveOccurred())
					Expect(IsScaleTimeOutError(err)).To(BeTrue())
					Expect(result).To(BeFalse())
				})
			})
			Context("When a scaled node is not ready", func() {
				It("Indicates that scaling has not completed", func() {
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
					result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
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
							ObjectMeta: metav1.ObjectMeta{
								Name:              testMachineName,
								Namespace:         testMachineNamespace,
								CreationTimestamp: metav1.Time{Time: time.Now()},
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
							ObjectMeta: metav1.ObjectMeta{
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
							ObjectMeta: metav1.ObjectMeta{
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
							ObjectMeta: metav1.ObjectMeta{
								Name:      testMachineSet,
								Namespace: "openshift-machine-api",
							},
						},
					},
				}
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
				result, err := scaler.EnsureScaleUpNodes(mockKubeClient, testDuration, logger)
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
					{ObjectMeta: metav1.ObjectMeta{Name: "scaled1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "scaled2"}},
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
				result, err := scaler.EnsureScaleDownNodes(mockKubeClient, logger)
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
				result, err := scaler.EnsureScaleDownNodes(mockKubeClient, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})
		It("Deletes all scaled machines", func() {
			var replicas int32 = 1
			originalMachineSets := &machineapi.MachineSetList{
				Items: []machineapi.MachineSet{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-machineset",
							Namespace: "openshift-machine-api",
						},
						Spec: machineapi.MachineSetSpec{
							Replicas: &replicas,
						},
					},
				},
			}
			nodes := &corev1.NodeList{
				Items: []corev1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"node-role.kubernetes.io/master": "",
							},
						},
						Status: corev1.NodeStatus{
							Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace("openshift-machine-api"),
					client.MatchingLabels{LABEL_UPGRADE: "true"},
				}).SetArg(1, *upgradeMachinesets).Times(1),
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
					}).Times(2),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					client.InNamespace("openshift-machine-api"),
					NotMatchingLabels{LABEL_UPGRADE: "true"},
				}).SetArg(1, *originalMachineSets).Times(1),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), []client.ListOption{
					NotMatchingLabels{"node-role.kubernetes.io/master": ""},
				}).SetArg(1, *nodes).Times(1),
			)
			result, err := scaler.EnsureScaleDownNodes(mockKubeClient, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})
})

package osd_cluster_upgrader

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/scaler"
	mockScaler "github.com/openshift/managed-upgrade-operator/pkg/scaler/mocks"
	"github.com/openshift/managed-upgrade-operator/util"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var stepCounter map[upgradev1alpha1.UpgradeConditionType]int
var _ = Describe("ClusterUpgrader", func() {
	var (
		logger logr.Logger
		// mocks
		mockKubeClient    *mocks.MockClient
		mockCtrl          *gomock.Controller
		mockMaintClient   *mockMaintenance.MockMaintenance
		mockScalerClient  *mockScaler.MockScaler
		mockMetricsClient *mockMetrics.MockMetrics
		// upgradeconfig to be used during tests
		upgradeConfigName types.NamespacedName
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		// clusterversion indicating a cluster hasn't yet commenced upgrading
		preUpgradeCV *configv1.ClusterVersionList
		config       *osdUpgradeConfig
	)

	BeforeEach(func() {
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		preUpgradeCV = &configv1.ClusterVersionList{
			Items: []configv1.ClusterVersion{{
				Spec: configv1.ClusterVersionSpec{
					DesiredUpdate: &configv1.Update{Version: upgradeConfig.Spec.Desired.Version + "different"},
					Channel:       upgradeConfig.Spec.Desired.Channel,
				},
				Status: configv1.ClusterVersionStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:   configv1.OperatorAvailable,
							Status: configv1.ConditionTrue,
						},
					},
					AvailableUpdates: []configv1.Update{
						{
							Version: upgradeConfig.Spec.Desired.Version,
							Image:   "quay.io/this-doesnt-exist",
							Force:   false,
						},
					},
				},
			}}}
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockMaintClient = mockMaintenance.NewMockMaintenance(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockScalerClient = mockScaler.NewMockScaler(mockCtrl)
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

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("When assessing if the control plane is upgraded to a version", func() {
		Context("When the clusterversion can't be fetched", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				util.ExpectGetClusterVersion(mockKubeClient, nil, fakeError)
				result, err := ControlPlaneUpgraded(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})

		Context("When that version is recorded in clusterversion's history", func() {
			var clusterVersionList *configv1.ClusterVersionList
			BeforeEach(func() {
				clusterVersionList = &configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{
						{
							Status: configv1.ClusterVersionStatus{
								History: []configv1.UpdateHistory{
									{State: configv1.CompletedUpdate, Version: "something"},
									{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version, StartedTime: metav1.Time{Time: time.Now()}},
									{State: configv1.CompletedUpdate, Version: "something else"},
								},
							},
						},
					},
				}
			})
			It("Flags the control plane as upgraded", func() {
				util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().IsMetricControlPlaneEndTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockMetricsClient.EXPECT().UpdateMetricControlPlaneEndTime(gomock.Any(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockMetricsClient.EXPECT().ResetMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := ControlPlaneUpgraded(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("When the control plane hasn't upgraded within the window", func() {
			var clusterVersionList *configv1.ClusterVersionList
			upgradeStartTime := time.Now().Add(-300 * time.Minute)
			BeforeEach(func() {
				clusterVersionList = &configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{
						{
							Status: configv1.ClusterVersionStatus{
								History: []configv1.UpdateHistory{
									{State: configv1.PartialUpdate, Version: upgradeConfig.Spec.Desired.Version, StartedTime: metav1.Time{Time: upgradeStartTime}},
								},
							},
						},
					},
				}
			})
			It("Sets the appropriate metric", func() {
				util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().UpdateMetricUpgradeControlPlaneTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := ControlPlaneUpgraded(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("Scaling", func() {
		It("Should scale up extra nodes and set success metric on successful scaling", func() {
			mockScalerClient.EXPECT().EnsureScaleUpNodes(gomock.Any(), config.GetScaleDuration(), gomock.Any()).Return(true, nil)
			mockMetricsClient.EXPECT().UpdateMetricScalingSucceeded(gomock.Any())
			expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
			ok, err := EnsureExtraUpgradeWorkers(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(Not(HaveOccurred()))
			Expect(ok).To(BeTrue())
		})
		It("Should set failed metric on scaling time out", func() {
			mockScalerClient.EXPECT().EnsureScaleUpNodes(gomock.Any(), config.GetScaleDuration(), gomock.Any()).Return(false, scaler.NewScaleTimeOutError("test scale timed out"))
			mockMetricsClient.EXPECT().UpdateMetricScalingFailed(gomock.Any())
			expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
			ok, err := EnsureExtraUpgradeWorkers(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(ok).To(BeFalse())
		})
	})

	Context("When requesting the cluster to begin upgrading", func() {
		Context("When the clusterversion version can't be fetched", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("a fake error")
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, fakeError)
				result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())
			})
		})

		Context("When the cluster is not on the same channel as the UpgradeConfig", func() {
			It("Updates the cluster's update channel", func() {
				clusterVersionList := &configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{
						{
							Spec: configv1.ClusterVersionSpec{
								Channel: upgradeConfig.Spec.Desired.Channel + "not-the-same",
								DesiredUpdate: nil,
							},
						},
					},
				}
				expectUpgradeCommenced(mockKubeClient, clusterVersionList, nil)
				util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
				gomock.InOrder(
					mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, cv *configv1.ClusterVersion) error {
							Expect(cv.Spec.Channel).To(Equal(upgradeConfig.Spec.Desired.Channel))
							return nil
						}),
				)
				result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())

			})
		})

		Context("When the cluster's desired version matches the UpgradeConfig's", func() {
			It("Indicates the upgrade has commenced", func() {
				expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, nil)
				mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(0)
				result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When the cluster's desired version is missing", func() {
			It("Sets the desired version to that of the UpgradeConfig's", func() {
				clusterVersionList := &configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{
						{
							Spec: configv1.ClusterVersionSpec{
								Channel: upgradeConfig.Spec.Desired.Channel,
								DesiredUpdate: nil,
							},
							Status: configv1.ClusterVersionStatus{
								AvailableUpdates: []configv1.Update{
									{
										Version: upgradeConfig.Spec.Desired.Version,
										Image:   "quay.io/dummy-image-for-test",
										Force:   false,
									},
								},
							},
						},
					},
				}
				expectUpgradeCommenced(mockKubeClient, clusterVersionList, nil)
				util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().IsMetricUpgradeStartTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, cv *configv1.ClusterVersion) error {
							Expect(cv.Spec.DesiredUpdate.Version).To(Equal(upgradeConfig.Spec.Desired.Version))
							Expect(cv.Spec.Channel).To(Equal(upgradeConfig.Spec.Desired.Channel))
							return nil
						}),
					mockMetricsClient.EXPECT().UpdateMetricUpgradeStartTime(gomock.Any(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("When the cluster's desired version does not match the UpgradeConfig's", func() {
			It("Sets the desired version to that of the UpgradeConfig's", func() {
				clusterVersionList := &configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{
						{
							Spec: configv1.ClusterVersionSpec{
								Channel: upgradeConfig.Spec.Desired.Channel,
								DesiredUpdate: &configv1.Update{
									Version: "something different",
								},
							},
							Status: configv1.ClusterVersionStatus{
								AvailableUpdates: []configv1.Update{
									{
										Version: upgradeConfig.Spec.Desired.Version,
										Image:   "quay.io/dummy-image-for-test",
										Force:   false,
									},
								},
							},
						},
					},
				}
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().IsMetricUpgradeStartTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, cv *configv1.ClusterVersion) error {
							Expect(cv.Spec.DesiredUpdate.Version).To(Equal(upgradeConfig.Spec.Desired.Version))
							Expect(cv.Spec.Channel).To(Equal(upgradeConfig.Spec.Desired.Channel))
							return nil
						}),
					mockMetricsClient.EXPECT().UpdateMetricUpgradeStartTime(gomock.Any(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})

		Context("When updating the clusterversion fails", func() {
			It("Indicates an error", func() {
				fakeError := fmt.Errorf("fake error")
				expectUpgradeHasNotCommenced(mockKubeClient, upgradeConfig, nil)
				util.ExpectGetClusterVersion(mockKubeClient, preUpgradeCV, nil)
				gomock.InOrder(
					mockMetricsClient.EXPECT().IsMetricUpgradeStartTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError),
				)
				result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).To(HaveOccurred())
				Expect(err).To(Equal(fakeError))
				Expect(result).To(BeFalse())

			})
		})
	})

	Context("When assessing whether all workers are upgraded", func() {
		var configPool *machineconfigapi.MachineConfigPool
		Context("When all workers are upgraded", func() {
			JustBeforeEach(func() {
				configPool = &machineconfigapi.MachineConfigPool{
					Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 3, UpdatedMachineCount: 3},
				}
			})
			It("Indicates that all workers are upgraded", func() {
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).Times(2),
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "worker"}, gomock.Any()).SetArg(2, *configPool),
					mockMaintClient.EXPECT().IsActive(),
					mockMetricsClient.EXPECT().IsMetricNodeUpgradeEndTimeSet(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockMetricsClient.EXPECT().UpdateMetricNodeUpgradeEndTime(gomock.Any(), upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
					mockMetricsClient.EXPECT().ResetMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := AllWorkersUpgraded(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeTrue())
			})
		})
		Context("When all workers are not upgraded", func() {
			JustBeforeEach(func() {
				configPool = &machineconfigapi.MachineConfigPool{
					Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 3, UpdatedMachineCount: 2},
				}
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "worker"}, gomock.Any()).SetArg(2, *configPool)
			})
			It("Indicates that all workers are not upgraded", func() {
				gomock.InOrder(
					mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).Times(2),
					mockMaintClient.EXPECT().IsActive(),
					mockMetricsClient.EXPECT().UpdateMetricUpgradeWorkerTimeout(upgradeConfig.Name, upgradeConfig.Spec.Desired.Version),
				)
				result, err := AllWorkersUpgraded(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeFalse())
			})
		})
	})

	Context("When the cluster's upgrade process has commenced", func() {
		It("will not re-perform a pre-upgrade health check", func() {
			expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, nil)
			result, err := PreClusterHealthCheck(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("will not re-perform spinning up extra workers", func() {
			expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, nil)
			result, err := EnsureExtraUpgradeWorkers(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
		It("will not re-perform commencing an upgrade", func() {
			expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, nil)
			result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})
	})

	Context("When the upgrader can't tell if the cluster's upgrade has commenced", func() {
		var fakeError = fmt.Errorf("fake upgradeCommenced error")
		It("will abort the pre-upgrade health check", func() {
			expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, fakeError)
			result, err := PreClusterHealthCheck(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
		It("will abort the spinning up of extra workers", func() {
			expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, fakeError)
			result, err := EnsureExtraUpgradeWorkers(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
		It("will abort the commencing of an upgrade", func() {
			expectUpgradeHasCommenced(mockKubeClient, upgradeConfig, fakeError)
			result, err := CommenceUpgrade(mockKubeClient, config, mockScalerClient, mockMetricsClient, mockMaintClient, upgradeConfig, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(fakeError))
			Expect(result).To(BeFalse())
		})
	})

	Context("When performing Cluster Upgrade steps", func() {
		var testSteps UpgradeSteps
		var testOrder UpgradeStepOrdering
		var cu *osdClusterUpgrader
		var step1 = upgradev1alpha1.UpgradeValidated
		BeforeEach(func() {
			testOrder = []upgradev1alpha1.UpgradeConditionType{
				step1,
			}
			testSteps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{
				step1: makeMockSucceedStep(step1),
			}
			cu = &osdClusterUpgrader{
				Steps:       testSteps,
				Ordering:    testOrder,
				client:      mockKubeClient,
				maintenance: mockMaintClient,
				metrics:     mockMetricsClient,
			}
			upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
				{
					Version: upgradeConfig.Spec.Desired.Version,
					Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
				},
			}
		})

		Context("When a step does not occur in the history", func() {
			BeforeEach(func() {
				cu.Steps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{
					step1: makeMockUnsucceededStep(step1),
				}
			})

			It("returns an uncompleted condition for the step", func() {
				// Add a step that will not complete on execution, so we can observe it starting
				phase, condition, err := cu.UpgradeCluster(upgradeConfig, logger)
				stepHistoryReason := condition.Reason
				Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgrading))
				Expect(condition.Status).To(Equal(corev1.ConditionFalse))
				Expect(stepHistoryReason).To(Equal(string(step1) + " not done"))
				Expect(err).NotTo(HaveOccurred())
			})

			It("runs the step", func() {
				_, _, err := cu.UpgradeCluster(upgradeConfig, logger)
				Expect(stepCounter[step1]).To(Equal(1))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("When running a step returns an error", func() {
			BeforeEach(func() {
				cu.Steps = map[upgradev1alpha1.UpgradeConditionType]UpgradeStep{
					step1: makeMockFailedStep(step1),
				}
			})
			It("Indicates the error in the condition", func() {
				_, condition, err := cu.UpgradeCluster(upgradeConfig, logger)
				stepHistoryReason := condition.Reason
				stepHistoryMsg := condition.Message
				Expect(stepHistoryReason).To(Equal(string(step1) + " not done"))
				Expect(stepHistoryMsg).To(Equal("step " + string(step1) + " failed"))
				Expect(stepCounter[step1]).To(Equal(1))
				Expect(err).To(HaveOccurred())
			})

		})

		Context("When all steps have indicated completion", func() {
			BeforeEach(func() {
				upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
					{
						Version: upgradeConfig.Spec.Desired.Version,
						Phase:   upgradev1alpha1.UpgradePhaseUpgrading,
						Conditions: []upgradev1alpha1.UpgradeCondition{
							{
								Type:    step1,
								Status:  corev1.ConditionTrue,
								Reason:  string(step1) + " succeed",
								Message: string(step1) + " succeed",
							},
						},
					},
				}
			})
			It("flags the upgrade as completed", func() {
				phase, condition, err := cu.UpgradeCluster(upgradeConfig, logger)
				Expect(phase).To(Equal(upgradev1alpha1.UpgradePhaseUpgraded))
				Expect(condition.Status).To(Equal(corev1.ConditionTrue))
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("Unit tests", func() {

		Context("When creating an UpgradeCondition", func() {
			It("Populates all fields properly", func() {
				reason := "testreason"
				msg := "testmsg"
				ucon := upgradev1alpha1.UpgradeConditionType("testuc")
				status := corev1.ConditionTrue
				uc := newUpgradeCondition(reason, msg, ucon, status)
				Expect(uc.Status).To(Equal(status))
				Expect(uc.Message).To(Equal(msg))
				Expect(uc.Reason).To(Equal(reason))
				Expect(uc.Type).To(Equal(ucon))
			})
		})

		Context("When assessing whether all machines are upgraded", func() {
			var configPool *machineconfigapi.MachineConfigPool
			var nodeType = "worker"

			Context("When getting the machine config pool fails", func() {
				It("Indicates that all machines aren't upgraded", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).Return(fmt.Errorf("Fake error"))
					result, err := nodesUpgraded(mockKubeClient, nodeType, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})

			Context("When all total machine and updated machine counts match", func() {
				JustBeforeEach(func() {
					configPool = &machineconfigapi.MachineConfigPool{
						Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 5, UpdatedMachineCount: 5},
					}
				})
				It("Reports that all nodes are upgraded", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).SetArg(2, *configPool)
					result, err := nodesUpgraded(mockKubeClient, nodeType, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeTrue())
				})
			})
			Context("When the updated machine count is less than the total machine count", func() {
				JustBeforeEach(func() {
					configPool = &machineconfigapi.MachineConfigPool{
						Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 3, UpdatedMachineCount: 2},
					}
				})
				It("Reports that all nodes are not upgraded", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).SetArg(2, *configPool)
					result, err := nodesUpgraded(mockKubeClient, nodeType, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})
			Context("When the updated machine count is greater than the total machine count", func() {
				JustBeforeEach(func() {
					configPool = &machineconfigapi.MachineConfigPool{
						Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 3, UpdatedMachineCount: 4},
					}
				})
				// TODO: Assess - does this make sense?
				It("Reports that all nodes are not upgraded", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).SetArg(2, *configPool)
					result, err := nodesUpgraded(mockKubeClient, nodeType, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeFalse())
				})
			})
		})

		Context("When getting the cluster's current version", func() {
			Context("When a completed update exists in the clusterversion history", func() {
				It("Returns that version", func() {
					version := "matchme"
					clusterVersion := &configv1.ClusterVersion{
						Status: configv1.ClusterVersionStatus{
							History: []configv1.UpdateHistory{
								{State: configv1.PartialUpdate, Version: "notmatch"},
								{State: configv1.CompletedUpdate, Version: version},
							},
						},
					}
					result, _ := GetCurrentVersion(clusterVersion)
					Expect(result).To(Equal(version))
				})
			})
			Context("When a completed update does not exist in the clusterversion history", func() {
				It("Returns an empty string", func() {
					clusterVersion := &configv1.ClusterVersion{
						Status: configv1.ClusterVersionStatus{
							History: []configv1.UpdateHistory{
								{State: configv1.PartialUpdate, Version: "notmatch"},
								{State: configv1.PartialUpdate, Version: "notmatch2"},
							},
						},
					}
					result, _ := GetCurrentVersion(clusterVersion)
					Expect(result).To(BeEmpty())
				})
			})
		})

	})

})

func makeMockSucceedStep(step upgradev1alpha1.UpgradeConditionType) UpgradeStep {
	return func(c client.Client, config *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
		stepCounter[step] += 1
		return true, nil
	}
}

func makeMockUnsucceededStep(step upgradev1alpha1.UpgradeConditionType) UpgradeStep {
	return func(c client.Client, config *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
		stepCounter[step] += 1
		return false, nil
	}
}

func makeMockFailedStep(step upgradev1alpha1.UpgradeConditionType) UpgradeStep {
	return func(c client.Client, config *osdUpgradeConfig, scaler scaler.Scaler, metricsClient metrics.Metrics, m maintenance.Maintenance, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (bool, error) {
		stepCounter[step] += 1
		return false, fmt.Errorf("step %s failed", step)
	}
}

func expectUpgradeHasNotCommenced(m *mocks.MockClient, u *upgradev1alpha1.UpgradeConfig, withErr error) {
	cvList := &configv1.ClusterVersionList{
		Items: []configv1.ClusterVersion{{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: u.Spec.Desired.Version + "different"},
				Channel:       u.Spec.Desired.Channel,
			},
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorAvailable,
						Status: configv1.ConditionTrue,
					},
				},
				AvailableUpdates: []configv1.Update{
					{
						Version: u.Spec.Desired.Version,
						Image:   "quay.io/dummy-image-for-test",
						Force:   false,
					},
				},
			},
		}}}
	expectUpgradeCommenced(m, cvList, withErr)
}

func expectUpgradeHasCommenced(m *mocks.MockClient, u *upgradev1alpha1.UpgradeConfig, withErr error) {
	cvList := &configv1.ClusterVersionList{
		Items: []configv1.ClusterVersion{{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{Version: u.Spec.Desired.Version},
				Channel:       u.Spec.Desired.Channel,
			},
			Status: configv1.ClusterVersionStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:   configv1.OperatorProgressing,
						Status: configv1.ConditionTrue,
					},
				},
				AvailableUpdates: []configv1.Update{
					{
						Version: u.Spec.Desired.Version,
						Image:   "quay.io/dummy-image-for-test",
						Force:   false,
					},
				},
			},
		}}}
	expectUpgradeCommenced(m, cvList, withErr)
}

func expectUpgradeCommenced(m *mocks.MockClient, cv *configv1.ClusterVersionList, withErr error) {
	cvList := m.EXPECT().List(gomock.Any(), gomock.Any())
	if cv != nil {
		cvList.SetArg(1, *cv)
	}
	if withErr != nil {
		cvList.Return(withErr)
	}
}

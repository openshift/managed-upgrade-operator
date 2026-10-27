package preflight

import (
	"context"
	"fmt"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	machineryMocks "github.com/openshift/managed-upgrade-operator/pkg/machinery/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	metricsMocks "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PreflightController", func() {
	var (
		mockCtrl              *gomock.Controller
		mockClient            *mocks.MockClient
		mockSubResourceClient *mocks.MockSubResourceClient
		mockConfigMgrBuilder  *configMocks.MockConfigManagerBuilder
		mockConfigMgr         *configMocks.MockConfigManager
		mockCvBuilder         *cvMocks.MockClusterVersionBuilder
		mockCv                *cvMocks.MockClusterVersion
		mockMetricsBuilder    *metricsMocks.MockMetricsBuilder
		mockMetrics           *metricsMocks.MockMetrics
		mockMachinery         *machineryMocks.MockMachinery

		reconciler *ReconcileUpgradePreflightCheck
		req        reconcile.Request
		instance   *upgradev1alpha1.UpgradePreflightCheck
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockSubResourceClient = mocks.NewMockSubResourceClient(mockCtrl)
		mockConfigMgrBuilder = configMocks.NewMockConfigManagerBuilder(mockCtrl)
		mockConfigMgr = configMocks.NewMockConfigManager(mockCtrl)
		mockCvBuilder = cvMocks.NewMockClusterVersionBuilder(mockCtrl)
		mockCv = cvMocks.NewMockClusterVersion(mockCtrl)
		mockMetricsBuilder = metricsMocks.NewMockMetricsBuilder(mockCtrl)
		mockMetrics = metricsMocks.NewMockMetrics(mockCtrl)
		mockMachinery = machineryMocks.NewMockMachinery(mockCtrl)

		reconciler = &ReconcileUpgradePreflightCheck{
			Client:               mockClient,
			MetricsClientBuilder: mockMetricsBuilder,
			CvClientBuilder:      mockCvBuilder,
			Machinery:            mockMachinery,
			ConfigManagerBuilder: mockConfigMgrBuilder,
		}

		req = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-preflight",
				Namespace: "openshift-managed-upgrade-operator",
			},
		}

		instance = &upgradev1alpha1.UpgradePreflightCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-preflight",
				Namespace: "openshift-managed-upgrade-operator",
			},
			Spec: upgradev1alpha1.UpgradePreflightCheckSpec{
				UpgradeConfigSpec: upgradev1alpha1.UpgradeConfigSpec{
					Type: "OSD",
					Desired: upgradev1alpha1.Update{
						Version: "4.16.15",
						Channel: "fast-4.16",
					},
				},
			},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	// gateConfig sets up ConfigManagerBuilder mock to return the given enabled feature gates.
	enableGate := func() {
		mockConfigMgrBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigMgr)
		mockConfigMgr.EXPECT().Into(gomock.Any()).DoAndReturn(func(cfg interface{}) error {
			c, ok := cfg.(*preflightGateConfig)
			if !ok {
				return fmt.Errorf("expected *preflightGateConfig, got %T", cfg)
			}
			c.FeatureGate.Enabled = []string{preflightControllerFeatureGate}
			return nil
		})
	}

	disableGate := func() {
		mockConfigMgrBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigMgr)
		mockConfigMgr.EXPECT().Into(gomock.Any()).Return(nil) // enabled list stays empty
	}

	Describe("Reconcile", func() {
		Context("when the feature gate is not enabled", func() {
			It("returns without error and does nothing", func() {
				disableGate()
				result, err := reconciler.Reconcile(context.TODO(), req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})

		Context("when the ConfigMap cannot be read", func() {
			It("returns without error (inactive)", func() {
				mockConfigMgrBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigMgr)
				mockConfigMgr.EXPECT().Into(gomock.Any()).Return(fmt.Errorf("configmap not found"))
				result, err := reconciler.Reconcile(context.TODO(), req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})

		Context("when the UpgradePreflightCheck CR is not found", func() {
			It("returns without error", func() {
				enableGate()
				mockClient.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Any()).
					Return(apierrors.NewNotFound(schema.GroupResource{}, "test-preflight"))
				result, err := reconciler.Reconcile(context.TODO(), req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})

		Context("when the CR is already completed", func() {
			It("returns without re-running checks", func() {
				enableGate()
				now := metav1.Now()
				instance.Status.CompletedAt = &now
				mockClient.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ types.NamespacedName, obj interface{}, _ ...interface{}) error {
						typedObj, ok := obj.(*upgradev1alpha1.UpgradePreflightCheck)
						if !ok {
							return fmt.Errorf("expected *UpgradePreflightCheck, got %T", obj)
						}
						*typedObj = *instance
						return nil
					})
				result, err := reconciler.Reconcile(context.TODO(), req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})

		Context("when the CR has not started yet", func() {
			It("sets StartedAt then runs checks", func() {
				enableGate()
				mockClient.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Any()).
					DoAndReturn(func(_ context.Context, _ types.NamespacedName, obj interface{}, _ ...interface{}) error {
						typedObj, ok := obj.(*upgradev1alpha1.UpgradePreflightCheck)
						if !ok {
							return fmt.Errorf("expected *UpgradePreflightCheck, got %T", obj)
						}
						*typedObj = *instance
						return nil
					})
				// StartedAt update
				mockClient.EXPECT().Status().Return(mockSubResourceClient)
				mockSubResourceClient.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

				// check calls (all checks enabled)
				mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv).AnyTimes()
				mockCv.EXPECT().GetClusterVersion().Return(&configv1.ClusterVersion{}, nil)
				mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetrics, nil)
				mockMetrics.EXPECT().Query(gomock.Any()).Return(&metrics.AlertResponse{}, nil)
				mockCv.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{}, nil)
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					Return(nil).Times(2) // ManuallyCordonedNodes + NodeUnschedulableTaints
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&policyv1.PodDisruptionBudgetList{}), gomock.Any()).
					Return(nil)

				// CompletedAt update
				mockClient.EXPECT().Status().Return(mockSubResourceClient)
				mockSubResourceClient.EXPECT().Update(gomock.Any(), gomock.Any()).Return(nil)

				result, err := reconciler.Reconcile(context.TODO(), req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
			})
		})
	})

	Describe("checkAllowed", func() {
		It("allows all checks when the allowlist is empty", func() {
			Expect(checkAllowed("CriticalAlerts", nil)).To(BeTrue())
			Expect(checkAllowed("ClusterOperators", []string{})).To(BeTrue())
		})

		It("allows only listed checks when allowlist is non-empty", func() {
			list := []string{"CriticalAlerts", "ClusterOperators"}
			Expect(checkAllowed("CriticalAlerts", list)).To(BeTrue())
			Expect(checkAllowed("ClusterOperators", list)).To(BeTrue())
			Expect(checkAllowed("PodDisruptionBudgets", list)).To(BeFalse())
		})
	})

	Describe("safeRun", func() {
		It("returns the check result normally", func() {
			result := safeRun("test", func() upgradev1alpha1.PreflightCheckResult {
				return upgradev1alpha1.PreflightCheckResult{
					Name:      "test",
					Execution: upgradev1alpha1.CheckExecution{State: upgradev1alpha1.CheckCompleted},
				}
			})
			Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
		})

		It("recovers from a panic and returns State=Error", func() {
			result := safeRun("test", func() upgradev1alpha1.PreflightCheckResult {
				panic("something went wrong")
			})
			Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
			Expect(result.Execution.Error).To(ContainSubstring("something went wrong"))
		})
	})

	Describe("checkTimeout", func() {
		It("returns the default when CheckTimeoutSeconds is 0", func() {
			Expect(checkTimeout(instance)).To(Equal(defaultCheckTimeout))
		})

		It("returns the spec value when set", func() {
			instance.Spec.CheckTimeoutSeconds = 60
			Expect(checkTimeout(instance)).To(Equal(60 * time.Second))
		})
	})

	Describe("pdbViolation", func() {
		It("returns empty string for a benign PDB", func() {
			pdb := policyv1.PodDisruptionBudget{}
			Expect(pdbViolation(pdb)).To(BeEmpty())
		})

		It("detects MaxUnavailable=0 (int)", func() {
			zero := intstr.FromInt(0)
			pdb := policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &zero},
			}
			Expect(pdbViolation(pdb)).To(Equal("MaxUnavailable=0"))
		})

		It("allows MaxUnavailable=1 (int)", func() {
			one := intstr.FromInt(1)
			pdb := policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &one},
			}
			Expect(pdbViolation(pdb)).To(BeEmpty())
		})

		It("detects MaxUnavailable=0% (string)", func() {
			zero := intstr.FromString("0%")
			pdb := policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &zero},
			}
			Expect(pdbViolation(pdb)).To(Equal("MaxUnavailable=0%"))
		})

		It("allows MaxUnavailable=10% (string)", func() {
			pct := intstr.FromString("10%")
			pdb := policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &pct},
			}
			Expect(pdbViolation(pdb)).To(BeEmpty())
		})

		It("detects MinAvailable=100% (string)", func() {
			full := intstr.FromString("100%")
			pdb := policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: &full},
			}
			Expect(pdbViolation(pdb)).To(Equal("MinAvailable=100%"))
		})

		It("allows MinAvailable=50% (string)", func() {
			half := intstr.FromString("50%")
			pdb := policyv1.PodDisruptionBudget{
				Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: &half},
			}
			Expect(pdbViolation(pdb)).To(BeEmpty())
		})
	})

	Describe("individual check functions", func() {
		Context("checkClusterVersion", func() {
			It("returns Passed with the current version on success", func() {
				mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv)
				mockCv.EXPECT().GetClusterVersion().Return(&configv1.ClusterVersion{
					Status: configv1.ClusterVersionStatus{
						Desired: configv1.Release{Version: "4.16.10"},
					},
				}, nil)
				result := reconciler.checkClusterVersion(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckPassed))
				Expect(result.Result.Details).To(ContainElement("currentVersion=4.16.10"))
			})

			It("returns Error when GetClusterVersion fails", func() {
				mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv)
				mockCv.EXPECT().GetClusterVersion().Return(nil, fmt.Errorf("cvo unavailable"))
				result := reconciler.checkClusterVersion(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
				Expect(result.Execution.Error).To(ContainSubstring("cvo unavailable"))
			})
		})

		Context("checkCriticalAlerts", func() {
			It("returns Passed when no critical alerts are firing", func() {
				mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetrics, nil)
				mockMetrics.EXPECT().Query(gomock.Any()).Return(&metrics.AlertResponse{
					Data: metrics.AlertData{Result: []metrics.AlertResult{}},
				}, nil)
				result := reconciler.checkCriticalAlerts(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckPassed))
			})

			It("returns Failed with alert names when critical alerts are firing", func() {
				mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetrics, nil)
				mockMetrics.EXPECT().Query(gomock.Any()).Return(&metrics.AlertResponse{
					Data: metrics.AlertData{Result: []metrics.AlertResult{
						{Metric: map[string]string{"alertname": "KubeNodeNotReady"}},
						{Metric: map[string]string{"alertname": "KubeNodeNotReady"}}, // duplicate
						{Metric: map[string]string{"alertname": "ClusterOperatorDown"}},
					}},
				}, nil)
				result := reconciler.checkCriticalAlerts(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckFailed))
				Expect(result.Result.Details).To(ConsistOf("KubeNodeNotReady", "ClusterOperatorDown"))
			})

			It("returns Error when the metrics client cannot be built after retries", func() {
				prometheusRetryBase = 1 * time.Millisecond
				defer func() { prometheusRetryBase = 2 * time.Second }()
				mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).
					Return(nil, fmt.Errorf("prometheus unreachable")).
					Times(prometheusRetries)
				result := reconciler.checkCriticalAlerts(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
				Expect(result.Execution.Error).To(ContainSubstring("prometheus unreachable"))
			})

			It("returns Error when Query fails after retries", func() {
				prometheusRetryBase = 1 * time.Millisecond
				defer func() { prometheusRetryBase = 2 * time.Second }()
				mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetrics, nil).Times(prometheusRetries)
				mockMetrics.EXPECT().Query(gomock.Any()).Return(nil, fmt.Errorf("query error")).Times(prometheusRetries)
				result := reconciler.checkCriticalAlerts(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
				Expect(result.Execution.Error).To(ContainSubstring("query error"))
			})
		})

		Context("checkClusterOperators", func() {
			It("returns Passed when no operators are degraded", func() {
				mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv)
				mockCv.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{
					Degraded: []string{},
				}, nil)
				result := reconciler.checkClusterOperators(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckPassed))
			})

			It("returns Failed with degraded operator names", func() {
				mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv)
				mockCv.EXPECT().HasDegradedOperators().Return(&clusterversion.HasDegradedOperatorsResult{
					Degraded: []string{"kube-apiserver", "etcd"},
				}, nil)
				result := reconciler.checkClusterOperators(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckFailed))
				Expect(result.Result.Details).To(ConsistOf("kube-apiserver", "etcd"))
			})

			It("returns Error when HasDegradedOperators fails", func() {
				mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv)
				mockCv.EXPECT().HasDegradedOperators().Return(nil, fmt.Errorf("co list failed"))
				result := reconciler.checkClusterOperators(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
				Expect(result.Execution.Error).To(ContainSubstring("co list failed"))
			})
		})

		Context("checkManuallyCordonedNodes", func() {
			workerNode := func(name string) corev1.Node {
				return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
			}

			It("returns Passed when no worker nodes are cordoned", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					DoAndReturn(func(_ context.Context, list interface{}, _ ...interface{}) error {
						nl, ok := list.(*corev1.NodeList)
						if !ok {
							return fmt.Errorf("expected *corev1.NodeList, got %T", list)
						}
						nl.Items = []corev1.Node{workerNode("worker-1")}
						return nil
					})
				mockMachinery.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: false})
				result := reconciler.checkManuallyCordonedNodes(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckPassed))
			})

			It("returns Failed when a worker node is manually cordoned", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					DoAndReturn(func(_ context.Context, list interface{}, _ ...interface{}) error {
						nl, ok := list.(*corev1.NodeList)
						if !ok {
							return fmt.Errorf("expected *corev1.NodeList, got %T", list)
						}
						nl.Items = []corev1.Node{workerNode("worker-1"), workerNode("worker-2")}
						return nil
					})
				// worker-1: cordoned by upgrader (skip), worker-2: cordoned manually (flag)
				mockMachinery.EXPECT().IsNodeCordoned(gomock.Any()).Return(&machinery.IsCordonedResult{IsCordoned: true}).Times(2)
				mockMachinery.EXPECT().IsNodeUpgrading(gomock.Any()).Return(true)  // worker-1: upgrading, skip
				mockMachinery.EXPECT().IsNodeUpgrading(gomock.Any()).Return(false) // worker-2: manual
				result := reconciler.checkManuallyCordonedNodes(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckFailed))
				Expect(result.Result.Details).To(ContainElement("worker-2"))
			})

			It("returns Error when node list fails", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					Return(fmt.Errorf("list nodes failed"))
				result := reconciler.checkManuallyCordonedNodes(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
				Expect(result.Execution.Error).To(ContainSubstring("list nodes failed"))
			})
		})

		Context("checkNodeTaints", func() {
			It("returns Passed when no nodes have pressure taints", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					DoAndReturn(func(_ context.Context, list interface{}, _ ...interface{}) error {
						nl, ok := list.(*corev1.NodeList)
						if !ok {
							return fmt.Errorf("expected *corev1.NodeList, got %T", list)
						}
						nl.Items = []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}}
						return nil
					})
				mockMachinery.EXPECT().HasMemoryPressure(gomock.Any()).Return(false)
				mockMachinery.EXPECT().HasDiskPressure(gomock.Any()).Return(false)
				mockMachinery.EXPECT().HasPidPressure(gomock.Any()).Return(false)
				result := reconciler.checkNodeTaints(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckPassed))
			})

			It("returns Failed when a node has memory pressure", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					DoAndReturn(func(_ context.Context, list interface{}, _ ...interface{}) error {
						nl, ok := list.(*corev1.NodeList)
						if !ok {
							return fmt.Errorf("expected *corev1.NodeList, got %T", list)
						}
						nl.Items = []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}}
						return nil
					})
				mockMachinery.EXPECT().HasMemoryPressure(gomock.Any()).Return(true)
				result := reconciler.checkNodeTaints(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckFailed))
				Expect(result.Result.Details).To(ContainElement("worker-1"))
			})

			It("returns Error when node list fails", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.NodeList{}), gomock.Any()).
					Return(fmt.Errorf("list nodes failed"))
				result := reconciler.checkNodeTaints(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
			})
		})

		Context("checkPodDisruptionBudgets", func() {
			It("returns Passed when no blocking PDBs exist", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&policyv1.PodDisruptionBudgetList{}), gomock.Any()).
					Return(nil)
				result := reconciler.checkPodDisruptionBudgets(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckPassed))
			})

			It("returns Failed for PDBs with MaxUnavailable=0", func() {
				zero := intstr.FromInt(0)
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&policyv1.PodDisruptionBudgetList{}), gomock.Any()).
					DoAndReturn(func(_ context.Context, list interface{}, _ ...interface{}) error {
						pl, ok := list.(*policyv1.PodDisruptionBudgetList)
						if !ok {
							return fmt.Errorf("expected *policyv1.PodDisruptionBudgetList, got %T", list)
						}
						pl.Items = []policyv1.PodDisruptionBudget{
							{
								ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "strict-pdb"},
								Spec:       policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &zero},
							},
						}
						return nil
					})
				result := reconciler.checkPodDisruptionBudgets(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckCompleted))
				Expect(result.Result.Verdict).To(Equal(upgradev1alpha1.CheckFailed))
				Expect(result.Result.Details).To(ContainElement(ContainSubstring("strict-pdb")))
			})

			It("returns Error when PDB list fails", func() {
				mockClient.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&policyv1.PodDisruptionBudgetList{}), gomock.Any()).
					Return(fmt.Errorf("list pdb failed"))
				result := reconciler.checkPodDisruptionBudgets(context.TODO(), log, defaultCheckTimeout)
				Expect(result.Execution.State).To(Equal(upgradev1alpha1.CheckError))
			})
		})
	})

	Describe("runChecks with allowlist", func() {
		It("skips checks not in the allowlist and runs those that are", func() {
			instance.Spec.Checks = []string{"ClusterVersion"}

			mockCvBuilder.EXPECT().New(gomock.Any()).Return(mockCv)
			mockCv.EXPECT().GetClusterVersion().Return(&configv1.ClusterVersion{}, nil)

			results := reconciler.runChecks(context.TODO(), instance, log)
			Expect(results).To(HaveLen(len(allChecks)))

			var skipped, completed, errored int
			for _, r := range results {
				switch r.Execution.State {
				case upgradev1alpha1.CheckSkipped:
					skipped++
				case upgradev1alpha1.CheckCompleted:
					completed++
				case upgradev1alpha1.CheckError:
					errored++
				}
			}
			_ = errored
			Expect(completed).To(Equal(1))
			Expect(skipped).To(Equal(len(allChecks) - 1))
		})
	})
})

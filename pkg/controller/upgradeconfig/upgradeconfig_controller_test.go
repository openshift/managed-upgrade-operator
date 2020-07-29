package upgradeconfig

import (
	"fmt"
	"testing"
	"time"

	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"
	"github.com/onsi/gomega/gstruct"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	mockUpgrader "github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader_builder/mocks"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	validationMocks "github.com/openshift/managed-upgrade-operator/pkg/validation/mocks"
	"github.com/openshift/managed-upgrade-operator/util"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	k8serrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeConfigController", func() {
	var (
		upgradeConfigName          types.NamespacedName
		upgradeConfig              *upgradev1alpha1.UpgradeConfig
		reconciler                 *ReconcileUpgradeConfig
		mockKubeClient             *mocks.MockClient
		mockMetricsClient          *mockMetrics.MockMetrics
		mockMetricsBuilder         *mockMetrics.MockMetricsBuilder
		mockUpdater                *mocks.MockStatusWriter
		mockClusterUpgraderBuilder *mockUpgrader.MockClusterUpgraderBuilder
		mockClusterUpgrader        *mockUpgrader.MockClusterUpgrader
		mockCtrl                   *gomock.Controller
		mockValidationBuilder      *validationMocks.MockValidationBuilder
		mockValidator              *validationMocks.MockValidator
		mockConfigManagerBuilder   *configMocks.MockConfigManagerBuilder
		mockConfigManager          *configMocks.MockConfigManager
		testScheme                 *runtime.Scheme
		cfg                        config
	)

	BeforeEach(func() {
		var err error
		testScheme, err = buildScheme()
		Expect(err).NotTo(HaveOccurred())

		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockMetricsClient = mockMetrics.NewMockMetrics(mockCtrl)
		mockUpdater = mocks.NewMockStatusWriter(mockCtrl)
		mockMetricsBuilder = mockMetrics.NewMockMetricsBuilder(mockCtrl)
		mockClusterUpgraderBuilder = mockUpgrader.NewMockClusterUpgraderBuilder(mockCtrl)
		mockClusterUpgrader = mockUpgrader.NewMockClusterUpgrader(mockCtrl)
		mockValidationBuilder = validationMocks.NewMockValidationBuilder(mockCtrl)
		mockConfigManagerBuilder = configMocks.NewMockConfigManagerBuilder(mockCtrl)
		mockConfigManager = configMocks.NewMockConfigManager(mockCtrl)
		mockValidator = validationMocks.NewMockValidator(mockCtrl)
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		cfg = config{
			UpgradeWindow: upgradeWindow{
				TimeOut: 60,
			},
		}
	})

	JustBeforeEach(func() {
		reconciler = &ReconcileUpgradeConfig{
			mockKubeClient,
			testScheme,
			mockMetricsBuilder,
			mockClusterUpgraderBuilder,
			mockValidationBuilder,
			mockConfigManagerBuilder,
		}
	})

	Context("Reconcile", func() {

		BeforeEach(func() {
			gomock.InOrder(
				mockMetricsBuilder.EXPECT().NewClient(gomock.Any()).Return(mockMetricsClient, nil),
			)
		})

		Context("When an UpgradeConfig doesn't exist", func() {
			It("Returns without error", func() {
				notFound := k8serrs.NewNotFound(schema.GroupResource{}, upgradeConfigName.Name)
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Return(notFound)
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("When fetching an UpgradeConfig fails", func() {
			It("Requeues the request", func() {
				fakeError := k8serrs.NewInternalError(fmt.Errorf("a fake error"))
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1).Return(fakeError)
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).To(Equal(fakeError))
				// This doesn't make a great deal of sense to me, but it's what the
				// boilerplate operator code says/does
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("When an UpgradeConfig exists", func() {
			Context("When the desired version isn't in the UpgradeConfig's history", func() {
				var desiredVersion = "a new version"
				var existingVersion = "not the same as desired version"
				BeforeEach(func() {
					upgradeConfig.Spec.Desired.Version = desiredVersion
					upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
						{
							Version: existingVersion,
						},
					}
				})
				Context("When updating the UpdateConfig's history fails", func() {
					It("Returns an error", func() {
						fakeError := k8serrs.NewInternalError(fmt.Errorf("a fake error"))
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When the history is added to the UpgradeConfig", func() {
					var clusterVersionList *configv1.ClusterVersionList
					BeforeEach(func() {
						clusterVersionList = &configv1.ClusterVersionList{
							Items: []configv1.ClusterVersion{
								{
									Status: configv1.ClusterVersionStatus{
										History: []configv1.UpdateHistory{
											{State: configv1.CompletedUpdate, Version: "something"},
											{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
											{State: configv1.CompletedUpdate, Version: "something else"},
										},
									},
								},
							},
						}
					})

					It("Adds it successfully", func() {
						matcher := testStructs.NewUpgradeConfigMatcher()
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)

						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
						mockKubeClient.EXPECT().Status().Return(mockUpdater).Times(3)
						mockUpdater.EXPECT().Update(gomock.Any(), matcher).Times(3)
						mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil)
						mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
						mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any())
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg)
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager)
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil)
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil)
						_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(matcher.ActualUpgradeConfig.Status.History).To(ContainElement(
							gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{"Version": Equal(desiredVersion)})))
					})
				})
			})
		})

		Context("When inspecting the UpgradeConfig's phase", func() {
			var version = "a version"
			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = version
				upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{
					{
						Version: version,
					},
				}
			})

			Context("When the upgrade phase is New", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
				})

				Context("When the cluster is not ready to upgrade", func() {
					// TODO: This is never true at the moment - no readiness check implemented
				})

				Context("When the cluster is ready to upgrade", func() {
					var clusterVersionList *configv1.ClusterVersionList
					BeforeEach(func() {
						clusterVersionList = &configv1.ClusterVersionList{
							Items: []configv1.ClusterVersion{
								{
									Status: configv1.ClusterVersionStatus{
										History: []configv1.UpdateHistory{
											{State: configv1.CompletedUpdate, Version: "something"},
											{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
											{State: configv1.CompletedUpdate, Version: "something else"},
										},
									},
								},
							},
						}
					})
					It("Adds a new Upgrade history to the UpgradeConfig", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
						mockKubeClient.EXPECT().Status().Return(mockUpdater).Times(3)
						mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Times(3)
						mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil)
						mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
						mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any())
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg)
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager)
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil)
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{Message: "test passed"}, nil)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
						Expect(upgradeConfig.Status.History.GetHistory("a version")).To(Not(BeNil()))
					})
					It("Invokes the upgrader", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
						mockKubeClient.EXPECT().Status().Return(mockUpdater).Times(3)
						mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Times(3)
						mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil)
						mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
						mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any())
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg)
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager)
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil)
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgraded, &upgradev1alpha1.UpgradeCondition{Message: "test passed"}, nil)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseUpgraded).To(BeTrue())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Conditions[0].Message == "test passed").To(BeTrue())
					})
				})
			})

			Context("When the upgrade phase is Pending", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhasePending
				})

				Context("When the cluster is ready to upgrade", func() {
					Context("When a cluster upgrade client can't be built", func() {
						var clusterVersionList *configv1.ClusterVersionList
						BeforeEach(func() {
							clusterVersionList = &configv1.ClusterVersionList{
								Items: []configv1.ClusterVersion{
									{
										Status: configv1.ClusterVersionStatus{
											History: []configv1.UpdateHistory{
												{State: configv1.CompletedUpdate, Version: "something"},
												{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
												{State: configv1.CompletedUpdate, Version: "something else"},
											},
										},
									},
								},
							}
						})
						var fakeError = fmt.Errorf("a maintenance builder error")
						It("does not proceed with upgrading the cluster", func() {
							util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
							gomock.InOrder(
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(0),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							)
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(Equal(fakeError))
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})

					Context("When a cluster upgrade client can be built", func() {
						var clusterVersionList *configv1.ClusterVersionList
						BeforeEach(func() {
							clusterVersionList = &configv1.ClusterVersionList{
								Items: []configv1.ClusterVersion{
									{
										Status: configv1.ClusterVersionStatus{
											History: []configv1.UpdateHistory{
												{State: configv1.CompletedUpdate, Version: "something"},
												{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
												{State: configv1.CompletedUpdate, Version: "something else"},
											},
										},
									},
								},
							}
						})
						It("Invokes the upgrader", func() {
							util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
							mockKubeClient.EXPECT().Status().Return(mockUpdater).Times(3)
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Times(3)
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil)
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any())
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager)
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg)
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil)
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil)
							mockKubeClient.EXPECT().Status().AnyTimes().Return(mockUpdater)
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes()
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})

					Context("When invoking the upgrader fails", func() {
						var fakeError = fmt.Errorf("the upgrader failed")
						var clusterVersionList *configv1.ClusterVersionList
						BeforeEach(func() {
							clusterVersionList = &configv1.ClusterVersionList{
								Items: []configv1.ClusterVersion{
									{
										Status: configv1.ClusterVersionStatus{
											History: []configv1.UpdateHistory{
												{State: configv1.CompletedUpdate, Version: "something"},
												{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
												{State: configv1.CompletedUpdate, Version: "something else"},
											},
										},
									},
								},
							}
						})

						It("reacts accordingly", func() {
							util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
							mockKubeClient.EXPECT().Status().Return(mockUpdater).Times(3)
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Times(3)
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil)
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any())
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager)
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg)
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil)
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, fakeError)
							mockKubeClient.EXPECT().Status().AnyTimes().Return(mockUpdater)
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes()
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})
				})

				Context("When the current time is before the upgrade window", func() {
					var clusterVersionList *configv1.ClusterVersionList
					BeforeEach(func() {
						upgradeConfig.Spec.UpgradeAt = time.Now().Add(80 * time.Minute).Format(time.RFC3339)
						clusterVersionList = &configv1.ClusterVersionList{
							Items: []configv1.ClusterVersion{
								{
									Status: configv1.ClusterVersionStatus{
										History: []configv1.UpdateHistory{
											{State: configv1.CompletedUpdate, Version: "something"},
											{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
											{State: configv1.CompletedUpdate, Version: "something else"},
										},
									},
								},
							},
						}
					})
					It("does nothing", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name).Times(1),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When the current time is after the upgrade window", func() {
					var clusterVersionList *configv1.ClusterVersionList
					BeforeEach(func() {
						upgradeConfig.Spec.UpgradeAt = time.Now().Add(-80 * time.Minute).Format(time.RFC3339)
						clusterVersionList = &configv1.ClusterVersionList{
							Items: []configv1.ClusterVersion{
								{
									Status: configv1.ClusterVersionStatus{
										History: []configv1.UpdateHistory{
											{State: configv1.CompletedUpdate, Version: "something"},
											{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
											{State: configv1.CompletedUpdate, Version: "something else"},
										},
									},
								},
							},
						}
					})
					It("raises an appropriate alert", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowBreached(upgradeConfig.Name),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})
			})

			Context("When the upgrade phase is Upgrading", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUpgrading
				})

				Context("When a cluster upgrade client can't be built", func() {
					var fakeError = fmt.Errorf("a maintenance builder error")
					It("does not proceed with upgrading the cluster", func() {
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(0),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When a cluster upgrade client can be built", func() {
					It("proceeds with upgrading the cluster", func() {
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
							mockKubeClient.EXPECT().Status().AnyTimes().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes(),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When invoking the upgrader fails", func() {
					var fakeError = fmt.Errorf("the upgrader failed")
					It("reacts accordingly", func() {
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(1).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, fakeError),
							mockKubeClient.EXPECT().Status().AnyTimes().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).AnyTimes(),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})
			})

			Context("When the upgrade phase is Upgraded", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUpgraded
				})
				It("does nothing", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
					mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			Context("When the upgrade phase is Failed", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseFailed
				})
				It("does nothing", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
					mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			Context("When the upgrade phase is Unknown", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUnknown
				})
				It("does nothing", func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
					mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})
		})
	})
})

func buildScheme() (*runtime.Scheme, error) {
	testScheme := runtime.NewScheme()
	var schemeErrors *multierror.Error
	schemeErrors = multierror.Append(schemeErrors, configv1.Install(testScheme))
	schemeErrors = multierror.Append(schemeErrors, routev1.Install(testScheme))
	schemeErrors = multierror.Append(schemeErrors, machineapi.AddToScheme(testScheme))
	schemeErrors = multierror.Append(schemeErrors, machineconfigapi.Install(testScheme))
	schemeErrors = multierror.Append(schemeErrors, upgradev1alpha1.SchemeBuilder.AddToScheme(testScheme))
	return testScheme, schemeErrors.ErrorOrNil()
}

func TestIsReadyToUpgrade(t *testing.T) {

	tests := []struct {
		name          string
		upgradeConfig *upgradev1alpha1.UpgradeConfig
		metricsClient *mockMetrics.MockMetrics
		result        bool
	}{
		{
			name:          "it should be ready to upgrade if upgradeAt is 10 mins before now",
			upgradeConfig: testUpgradeConfig(true, time.Now().Add(-10*time.Minute).Format(time.RFC3339)),
			metricsClient: mockMetrics.NewMockMetrics(gomock.NewController(t)),
			result:        true,
		},
		{
			name:          "it should be not ready to upgrade if upgradeAt is 80 mins before now",
			upgradeConfig: testUpgradeConfig(true, time.Now().Add(80*time.Minute).Format(time.RFC3339)),
			metricsClient: mockMetrics.NewMockMetrics(gomock.NewController(t)),
			result:        false,
		},
		{
			name:          "it should not be ready to upgrade if proceed is set to false",
			upgradeConfig: testUpgradeConfig(false, time.Now().Format(time.RFC3339)),
			metricsClient: mockMetrics.NewMockMetrics(gomock.NewController(t)),
			result:        false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.metricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(gomock.Any()).AnyTimes()
			test.metricsClient.EXPECT().UpdateMetricUpgradeWindowBreached(gomock.Any()).AnyTimes()
			result := isReadyToUpgrade(test.upgradeConfig, test.metricsClient, 60*time.Minute)
			if result != test.result {
				t.Fail()
			}

		})
	}

}

func testUpgradeConfig(proceed bool, upgradeAt string) *upgradev1alpha1.UpgradeConfig {
	return &upgradev1alpha1.UpgradeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "upgradeconfig-example",
		},
		Spec: upgradev1alpha1.UpgradeConfigSpec{
			Proceed:   proceed,
			UpgradeAt: upgradeAt,
		},
	}
}

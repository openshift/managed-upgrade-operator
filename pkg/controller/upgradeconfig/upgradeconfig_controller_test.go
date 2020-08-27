package upgradeconfig

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"
	"github.com/onsi/gomega/gstruct"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	mockUpgrader "github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader_builder/mocks"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	schedulerMocks "github.com/openshift/managed-upgrade-operator/pkg/scheduler/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
	validationMocks "github.com/openshift/managed-upgrade-operator/pkg/validation/mocks"
	"github.com/openshift/managed-upgrade-operator/util"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	k8serrs "k8s.io/apimachinery/pkg/api/errors"

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
		mockScheduler              *schedulerMocks.MockScheduler
		testScheme                 *runtime.Scheme
		cfg                        config
		upgradingReconcileTime     time.Duration
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
		mockScheduler = schedulerMocks.NewMockScheduler(mockCtrl)
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
		upgradingReconcileTime = 1 * time.Minute
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	JustBeforeEach(func() {
		reconciler = &ReconcileUpgradeConfig{
			mockKubeClient,
			testScheme,
			mockMetricsBuilder,
			mockClusterUpgraderBuilder,
			mockValidationBuilder,
			mockConfigManagerBuilder,
			mockScheduler,
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
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Return(fakeError)
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
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
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

						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
						)
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
				var clusterVersionList *configv1.ClusterVersionList
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
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
				Context("When the upgradeconfig validation fails", func() {
					It("should set the validation alert metric", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)

						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: false, IsAvailableUpdate: false}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationFailed(gomock.Any()),
						)
						_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
					})
				})

				Context("When the cluster should not proceed with an upgrade", func() {
					It("should not attempt to upgrade", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)

						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:false}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
						)
						_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
					})
				})

				Context("When the cluster is not ready to upgrade", func() {
					It("should set status to pending", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)

						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false}),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
					})
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
					It("The configuration configmap must exist", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).Return(fmt.Errorf("config error")),
						)
						_, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal("config error"))
					})
					It("Adds a new Upgrade history to the UpgradeConfig", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{Message: "test passed"}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						Expect(upgradeConfig.Status.History.GetHistory("a version")).To(Not(BeNil()))
					})
					It("Invokes the upgrader", func() {
						util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgraded, &upgradev1alpha1.UpgradeCondition{Message: "test passed"}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
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
						var fakeError = fmt.Errorf("an upgrader builder error")
						It("does not proceed with upgrading the cluster", func() {
							util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
							gomock.InOrder(
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Times(0),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
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
							gomock.InOrder(
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
								mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
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
							gomock.InOrder(
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
								mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowNotBreached(upgradeConfig.Name),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, fakeError),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						})
					})
				})
			})

			Context("When the current time is before the upgrade window", func() {
				var clusterVersionList *configv1.ClusterVersionList
				BeforeEach(func() {
					upgradeConfig.Spec.UpgradeAt = time.Now().Add(80 * time.Minute).Format(time.RFC3339)
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
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
				It("sets the status to pending", func() {
					util.ExpectGetClusterVersion(mockKubeClient, clusterVersionList, nil)
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
						mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
						mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
						mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false}),
						mockKubeClient.EXPECT().Status().Return(mockUpdater),
						mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
					)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
					Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
				})
			})

			Context("When the current time is after the upgrade window", func() {
				var clusterVersionList *configv1.ClusterVersionList
				BeforeEach(func() {
					upgradeConfig.Spec.UpgradeAt = time.Now().Add(-80 * time.Minute).Format(time.RFC3339)
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
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
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
						mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate:true}, nil),
						mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
						mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false, IsBreached: true}),
						mockMetricsClient.EXPECT().UpdateMetricUpgradeWindowBreached(upgradeConfig.Name),
						mockKubeClient.EXPECT().Status().Return(mockUpdater),
						mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
					)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
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
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
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
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
					})
				})

				Context("When invoking the upgrader fails", func() {
					var fakeError = fmt.Errorf("the upgrader failed")
					It("reacts accordingly", func() {
						gomock.InOrder(
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, fakeError),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
					})
				})
			})

			Context("When the upgrade phase is Upgraded", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUpgraded
				})
				It("does nothing", func() {
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0),
					)
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
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0),
					)
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
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0),
					)
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

package upgradeconfig

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega/gstruct"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	emMocks "github.com/openshift/managed-upgrade-operator/pkg/eventmanager/mocks"
	mockMetrics "github.com/openshift/managed-upgrade-operator/pkg/metrics/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/scheduler"
	schedulerMocks "github.com/openshift/managed-upgrade-operator/pkg/scheduler/mocks"
	ucMgrMocks "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	mockUpgrader "github.com/openshift/managed-upgrade-operator/pkg/upgraders/mocks"
	"github.com/openshift/managed-upgrade-operator/pkg/validation"
	validationMocks "github.com/openshift/managed-upgrade-operator/pkg/validation/mocks"
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
		mockCVClientBuilder        *cvMocks.MockClusterVersionBuilder
		mockCVClient               *cvMocks.MockClusterVersion
		mockEMBuilder              *emMocks.MockEventManagerBuilder
		mockEMClient               *emMocks.MockEventManager
		mockUCMgrBuilder           *ucMgrMocks.MockUpgradeConfigManagerBuilder
		mockUCMgr                  *ucMgrMocks.MockUpgradeConfigManager
		testScheme                 *runtime.Scheme
		cfg                        config
		upgradingReconcileTime     time.Duration
	)

	BeforeEach(func() {
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
		mockCVClientBuilder = cvMocks.NewMockClusterVersionBuilder(mockCtrl)
		mockCVClient = cvMocks.NewMockClusterVersion(mockCtrl)
		mockEMBuilder = emMocks.NewMockEventManagerBuilder(mockCtrl)
		mockEMClient = emMocks.NewMockEventManager(mockCtrl)
		mockUCMgrBuilder = ucMgrMocks.NewMockUpgradeConfigManagerBuilder(mockCtrl)
		mockUCMgr = ucMgrMocks.NewMockUpgradeConfigManager(mockCtrl)
		upgradeConfigName = types.NamespacedName{
			Name:      "managed-upgrade-config",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		cfg = config{
			UpgradeWindow: upgradeWindow{
				TimeOut: 60,
			},
		}
		upgradingReconcileTime = 1 * time.Minute
		_ = os.Setenv("OPERATOR_NAMESPACE", "test-namespace")
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
			mockCVClientBuilder,
			mockEMBuilder,
			mockUCMgrBuilder,
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
				gomock.InOrder(
					mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Return(notFound),
					mockMetricsClient.EXPECT().ResetAllMetrics(),
				)
				result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("When fetching an UpgradeConfig fails", func() {
			It("Requeues the request", func() {
				fakeError := k8serrs.NewInternalError(fmt.Errorf("a fake error"))
				gomock.InOrder(
					mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Return(fakeError),
				)
				result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
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
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When the history is added to the UpgradeConfig", func() {
					var clusterVersion *configv1.ClusterVersion
					BeforeEach(func() {
						clusterVersion = &configv1.ClusterVersion{
							Status: configv1.ClusterVersionStatus{
								History: []configv1.UpdateHistory{
									{State: configv1.CompletedUpdate, Version: "something"},
									{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
									{State: configv1.CompletedUpdate, Version: "something else"},
								},
							},
						}
					})

					It("Adds it successfully", func() {
						matcher := testStructs.NewUpgradeConfigMatcher()
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
							mockUCMgr.EXPECT().Refresh().Return(false, nil),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
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
				var clusterVersion *configv1.ClusterVersion
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
					clusterVersion = &configv1.ClusterVersion{
						Status: configv1.ClusterVersionStatus{
							History: []configv1.UpdateHistory{
								{State: configv1.CompletedUpdate, Version: "something"},
								{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
								{State: configv1.CompletedUpdate, Version: "something else"},
							},
						},
					}
				})
				Context("When the upgradeconfig validation fails", func() {
					It("should set the validation alert metric", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: false, IsAvailableUpdate: false}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationFailed(gomock.Any()),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
					})
				})

				Context("When the cluster should not proceed with an upgrade", func() {
					It("should not attempt to upgrade", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: false}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
					})
				})

				Context("When the cluster is not ready to upgrade", func() {
					It("should set status to pending", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false}),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
					})
				})

				Context("When the cluster is ready to upgrade", func() {
					var clusterVersion *configv1.ClusterVersion
					BeforeEach(func() {
						clusterVersion = &configv1.ClusterVersion{
							Status: configv1.ClusterVersionStatus{
								History: []configv1.UpdateHistory{
									{State: configv1.CompletedUpdate, Version: "something"},
									{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
									{State: configv1.CompletedUpdate, Version: "something else"},
								},
							},
						}
					})
					It("The configuration configmap must exist", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).Return(fmt.Errorf("config error")),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(Equal("config error"))
					})
					It("Adds a new Upgrade history to the UpgradeConfig", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
							mockUCMgr.EXPECT().Refresh().Return(false, nil),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{Message: "test passed"}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
					})
					It("Remote upgrade policy changed", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
							mockUCMgr.EXPECT().Refresh().Return(true, nil),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseNew).To(BeTrue())
					})
					It("Invokes the upgrader", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
							mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
							mockUCMgr.EXPECT().Refresh().Return(false, nil),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgraded, &upgradev1alpha1.UpgradeCondition{Message: "test passed"}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
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
						var clusterVersion *configv1.ClusterVersion
						BeforeEach(func() {
							clusterVersion = &configv1.ClusterVersion{
								Status: configv1.ClusterVersionStatus{
									History: []configv1.UpdateHistory{
										{State: configv1.CompletedUpdate, Version: "something"},
										{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
										{State: configv1.CompletedUpdate, Version: "something else"},
									},
								},
							}
						})
						var fakeError = fmt.Errorf("an upgrader builder error")
						It("does not proceed with upgrading the cluster", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Times(0),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(false, nil),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(Equal(fakeError))
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})

					Context("When a cluster upgrade client can be built", func() {
						var clusterVersion *configv1.ClusterVersion
						BeforeEach(func() {
							clusterVersion = &configv1.ClusterVersion{
								Status: configv1.ClusterVersionStatus{
									History: []configv1.UpdateHistory{
										{State: configv1.CompletedUpdate, Version: "something"},
										{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
										{State: configv1.CompletedUpdate, Version: "something else"},
									},
								},
							}
						})
						It("Invokes the upgrader", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(false, nil),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						})
					})

					Context("When invoking the upgrader fails", func() {
						var fakeError = fmt.Errorf("the upgrader failed")
						var clusterVersion *configv1.ClusterVersion
						BeforeEach(func() {
							clusterVersion = &configv1.ClusterVersion{
								Status: configv1.ClusterVersionStatus{
									History: []configv1.UpdateHistory{
										{State: configv1.CompletedUpdate, Version: "something"},
										{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
										{State: configv1.CompletedUpdate, Version: "something else"},
									},
								},
							}
						})

						It("reacts accordingly", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
								mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(false, nil),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, fakeError),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						})
					})
				})
			})

			Context("When the current time is before the upgrade window", func() {
				var clusterVersion *configv1.ClusterVersion
				BeforeEach(func() {
					upgradeConfig.Spec.UpgradeAt = time.Now().Add(80 * time.Minute).Format(time.RFC3339)
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
					clusterVersion = &configv1.ClusterVersion{
						Status: configv1.ClusterVersionStatus{
							History: []configv1.UpdateHistory{
								{State: configv1.CompletedUpdate, Version: "something"},
								{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
								{State: configv1.CompletedUpdate, Version: "something else"},
							},
						},
					}
				})
				It("sets the status to pending", func() {
					gomock.InOrder(
						mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
						mockCVClient.EXPECT().GetClusterVersion().Return(clusterVersion, nil),
						mockValidationBuilder.EXPECT().NewClient().Return(mockValidator, nil),
						mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
						mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
						mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false}),
						mockKubeClient.EXPECT().Status().Return(mockUpdater),
						mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
					)
					result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
					Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
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
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Times(0),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When a cluster upgrade client can be built", func() {
					It("proceeds with upgrading the cluster", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
					})
				})

				Context("When invoking the upgrader fails", func() {
					var fakeError = fmt.Errorf("the upgrader failed")
					It("reacts accordingly", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, &upgradev1alpha1.UpgradeCondition{}, fakeError),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
					})
				})
			})

			Context("When the upgrade phase is Failed", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseFailed
				})
				It("does nothing", func() {
					gomock.InOrder(
						mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0),
					)
					result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
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
						mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Times(0),
					)
					result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})
		})
	})
})

package upgradeconfig

import (
	"context"
	"fmt"
	"os"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"go.uber.org/mock/gomock"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cvMocks "github.com/openshift/managed-upgrade-operator/pkg/clusterversion/mocks"
	configMocks "github.com/openshift/managed-upgrade-operator/pkg/configmanager/mocks"
	dvomocks "github.com/openshift/managed-upgrade-operator/pkg/dvo/mocks"
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
	"github.com/onsi/gomega/gstruct"
)

const (
	TEST_CV_VERSION = "4.13.0"
	TEST_CV_CHANNEL = "stable-4.13"
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
		testClusterVersion         *configv1.ClusterVersion
		mockdvobuilder             *dvomocks.MockDvoClientBuilder
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
		mockdvobuilder = dvomocks.NewMockDvoClientBuilder(mockCtrl)
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
		testClusterVersion = &configv1.ClusterVersion{
			Spec: configv1.ClusterVersionSpec{
				DesiredUpdate: &configv1.Update{
					Version: TEST_CV_VERSION,
				},
				Channel: TEST_CV_CHANNEL,
			},
			Status: configv1.ClusterVersionStatus{
				History: []configv1.UpdateHistory{
					{State: configv1.CompletedUpdate, Version: "something"},
					{State: configv1.CompletedUpdate, Version: upgradeConfig.Spec.Desired.Version},
					{State: configv1.CompletedUpdate, Version: "something else"},
				},
			},
		}
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
			mockdvobuilder,
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
					mockMetricsClient.EXPECT().ResetEphemeralMetrics(),
				)
				result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).NotTo(HaveOccurred())
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
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("When attempting to fetch the configmap", func() {
			var version = "a version"
			var fakeError = fmt.Errorf("configmap not found")
			BeforeEach(func() {
				upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{{}}
				cfg = config{
					FeatureGate: featureGate{
						Enabled: []string{"PreHealthCheck"},
					},
				}
			})
			JustBeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = version
				upgradeConfig.Status.History[0].Version = version
			})
			It("The configuration configmap must exist", func() {
				gomock.InOrder(
					mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
					mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
					mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
					mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
					mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
					mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
				)
				_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).ToNot(HaveOccurred())
			})
			It("must report error if not found", func() {
				gomock.InOrder(
					mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
					mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
					mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
					mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
					mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
					mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg).Return(fakeError),
				)
				_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("When an UpgradeConfig exists", func() {
			Context("and there is no existing history", func() {
				Context("and the cluster is already upgrading to that version", func() {
					BeforeEach(func() {
						// set CVO's version to that of the UC's
						testClusterVersion.Spec.DesiredUpdate.Version = upgradeConfig.Spec.Desired.Version
						testClusterVersion.Status.Desired.Version = "PreviousVersion"
					})
					It("sets an appropriate history phase", func() {
						matcher := testStructs.NewUpgradeConfigMatcher()
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(matcher.ActualUpgradeConfig.Status.History).To(ContainElement(
							gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{"PrecedingVersion": Equal("PreviousVersion"), "Version": Equal(upgradeConfig.Spec.Desired.Version),
								"Phase": Equal(upgradev1alpha1.UpgradePhaseUpgrading)})))
					})
				})
			})

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
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(true, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When the history is added to the UpgradeConfig", func() {
					It("Adds it successfully", func() {
						matcher := testStructs.NewUpgradeConfigMatcher()
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockCVClient.EXPECT().HasUpgradeCommenced(gomock.Any()).Return(false, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), matcher),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{}),
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
				upgradeConfig.Status.History = []upgradev1alpha1.UpgradeHistory{{}}
			})
			JustBeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = version
				upgradeConfig.Status.History[0].Version = version
			})
			Context("When the UpgradePhase is New", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
					cfg = config{
						FeatureGate: featureGate{
							Enabled: []string{"PreHealthCheck"},
						},
					}
				})
				Context("When the time to upgrade is more than the HealthCheckDuration and feature flag is set", func() {
					sr := scheduler.SchedulerResult{
						IsReady:          false,
						IsBreached:       false,
						TimeUntilUpgrade: 5 * time.Hour,
					}
					It("Should run pre-health check", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
							mockClusterUpgrader.EXPECT().HealthCheck(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
					})

					var fakeError = fmt.Errorf("a healthcheck error")
					It("Should move to pending phase if the HealthCheck fails", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
							mockClusterUpgrader.EXPECT().HealthCheck(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, fakeError),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).ToNot(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
					})
				})
				Context("When the time to upgrade is more than the HealthCheckDuration and feature flag is not set", func() {
					sr := scheduler.SchedulerResult{
						IsReady:          false,
						IsBreached:       false,
						TimeUntilUpgrade: 5 * time.Hour,
					}
					It("Should not run pre-health check", func() {
						cfg = config{}
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
					})
				})
				Context("When the upgrade time is less than the healthcheck duration and feature flag is set", func() {
					sr := scheduler.SchedulerResult{
						IsReady:          false,
						IsBreached:       false,
						TimeUntilUpgrade: 1 * time.Hour,
					}
					It("Should skip prehealth check and move to pending phase", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
					})
				})
				Context("When the upgrade time is less than the healthcheck duration and feature flag is not set", func() {
					sr := scheduler.SchedulerResult{
						IsReady:          false,
						IsBreached:       false,
						TimeUntilUpgrade: 1 * time.Hour,
					}
					It("Should skip prehealth check and move to pending phase", func() {
						cfg = config{}
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
					})
				})
				Context("When a cluster upgrade client can't be built", func() {
					var fakeError = fmt.Errorf("an upgrader builder error")
					It("does not proceed with upgrading the cluster", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When the current time is before the upgrade window", func() {
					BeforeEach(func() {
						upgradeConfig.Spec.UpgradeAt = time.Now().Add(80 * time.Minute).Format(time.RFC3339)
						upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
					})
					It("sets the status to pending", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{TimeUntilUpgrade: 3 * time.Hour}),
							mockClusterUpgrader.EXPECT().HealthCheck(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())

						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
					})
				})
				Context("When the status update fails to set to pending phase", func() {
					BeforeEach(func() {
						upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
					})
					var fakeError = fmt.Errorf("a status update error")
					sr := scheduler.SchedulerResult{
						IsReady:          false,
						IsBreached:       false,
						TimeUntilUpgrade: 1 * time.Hour,
					}
					It("should report reconcile failure", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
					})
				})

			})

			Context("When the UpgradePhase is Pending", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhasePending
				})

				Context("When building the validator client fails", func() {
					var fakeError = fmt.Errorf("an validator builder error")
					It("reconcile should fail", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, fakeError),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
					})
				})

				Context("When the upgradeconfig validation fails", func() {
					It("should set the validation alert metric", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
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
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: false}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
					})
				})

				Context("When the cluster is ready to upgrade", func() {
					var fakeError = fmt.Errorf("fake upgradeconfig manager builder error")
					It("Should fail if cannot create upgradeconfig manager builder", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, fakeError),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
					})
					It("Adds a new Upgrade history to the UpgradeConfig", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
							mockUCMgr.EXPECT().Refresh().Return(false, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(time.Minute * 1))
					})
					Context("When remote upgrade policy is attempted to be fetched", func() {
						It("should reconcile and set phase to New phase if remote policy has changed", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(true, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.RequeueAfter).To(BeZero())
							Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseNew).To(BeTrue())
						})

						It("should reconcile error if status update for new phase fails after remote change", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(true, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fmt.Errorf("status update failure")),
							)
							_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseNew).To(BeTrue())
						})

						var fakeError = fmt.Errorf("fake remote config error")
						It("should reconcile with failure if error is other than remote config manager not configured", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(true, fakeError),
							)
							_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseNew).To(BeFalse())
						})

						fakeError = fmt.Errorf("fake error to set status update in history")
						It("should reconcile with failure if not able to update status for upgrading phase", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(false, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError),
							)
							_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseUpgrading).To(BeTrue())
						})
					})

					It("Invokes the upgrader", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
							mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
							mockUCMgr.EXPECT().Refresh().Return(false, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgraded, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhaseUpgraded).To(BeTrue())
					})

					Context("When a cluster upgrade client can be built", func() {
						It("Invokes the upgrader", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(false, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						})
					})

					Context("When invoking the upgrader fails", func() {
						var fakeError = fmt.Errorf("the upgrader failed")
						It("reacts accordingly", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: true}),
								mockUCMgrBuilder.EXPECT().NewManager(gomock.Any()).Return(mockUCMgr, nil),
								mockUCMgr.EXPECT().Refresh().Return(false, nil),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
								mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, fakeError),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
						})
					})
				})

				Context("When the cluster is not ready to upgrade", func() {

					It("Should update phase status to be pending phase", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
							mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
							mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
							mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false}),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).ToNot(HaveOccurred())
						Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
					})

					Context("When the status update fails to set to pending phase", func() {
						var statusError = fmt.Errorf("a status update error")
						It("Should reconcile error", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(scheduler.SchedulerResult{IsReady: false}),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(statusError),
							)
							_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(HaveOccurred())
							Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
						})
					})

					Context("When the time to upgrade is quite near to the next reconcile", func() {
						sr := scheduler.SchedulerResult{
							IsReady:    false,
							IsBreached: false,
							// We keep time until upgrade to be less than SyncPeriodDefault i.e, 5 minutes
							TimeUntilUpgrade: 3 * time.Minute,
						}
						It("Should reconcile based on the time until upgrade interval", func() {
							gomock.InOrder(
								mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
								mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
								mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
								mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
								mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
								mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
								mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
								mockValidationBuilder.EXPECT().NewClient(mockConfigManager).Return(mockValidator, nil),
								mockValidator.EXPECT().IsValidUpgradeConfig(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(validation.ValidatorResult{IsValid: true, IsAvailableUpdate: true}, nil),
								mockMetricsClient.EXPECT().UpdateMetricValidationSucceeded(gomock.Any()),
								mockScheduler.EXPECT().IsReadyToUpgrade(gomock.Any(), gomock.Any()).Return(sr),
								mockKubeClient.EXPECT().Status().Return(mockUpdater),
								mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
							)
							result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).ToNot(HaveOccurred())
							Expect(upgradeConfig.Status.History.GetHistory("a version").Phase == upgradev1alpha1.UpgradePhasePending).To(BeTrue())
							Expect(result.RequeueAfter).To(Equal(sr.TimeUntilUpgrade))
						})
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
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(nil, fakeError),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Times(0),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When a cluster upgrade client can be built", func() {
					It("proceeds with upgrading the cluster", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, nil),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
					})
				})

				Context("When invoking the upgrader fails", func() {
					var fakeError = fmt.Errorf("the upgrader failed")
					It("reacts accordingly", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any()).Return(upgradev1alpha1.UpgradePhaseUpgrading, fakeError),
							mockKubeClient.EXPECT().Status().Return(mockUpdater),
							mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(HaveOccurred())
						Expect(result.RequeueAfter).To(Equal(upgradingReconcileTime))
					})
				})
			})

			Context("When the upgrade phase is Upgraded", func() {
				BeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUpgraded
					upgradeConfig.Status.History[0].StartTime = &metav1.Time{Time: time.Now()}
					upgradeConfig.Status.History[0].CompleteTime = &metav1.Time{Time: time.Now()}

				})
				It("reports metrics", func() {
					gomock.InOrder(
						mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
						mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
						mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
						mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
						mockMetricsClient.EXPECT().AlertsFromUpgrade(gomock.Any(), gomock.Any()),
						mockMetricsClient.EXPECT().UpdateMetricUpgradeResult(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()),
					)
					result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.RequeueAfter).To(BeZero())
				})
				When("It's a minor version change", func() {
					BeforeEach(func() {
						version = "4.15.0"
						upgradeConfig.Status.History[0].PrecedingVersion = "4.14.0"
					})
					It("reports metric with stream = y", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockMetricsClient.EXPECT().AlertsFromUpgrade(gomock.Any(), gomock.Any()),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeResult(gomock.Any(), "4.14.0", "4.15.0", "y", gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})

						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(BeZero())

					})
				})

				When("It's a patch version change", func() {
					BeforeEach(func() {
						version = "4.15.2"
						upgradeConfig.Status.History[0].PrecedingVersion = "4.15.1"
					})
					It("reports metric with stream = z", func() {
						gomock.InOrder(
							mockEMBuilder.EXPECT().NewManager(gomock.Any()).Return(mockEMClient, nil),
							mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig),
							mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
							mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
							mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
							mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
							mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
							mockMetricsClient.EXPECT().AlertsFromUpgrade(gomock.Any(), gomock.Any()),
							mockMetricsClient.EXPECT().UpdateMetricUpgradeResult(gomock.Any(), "4.15.1", "4.15.2", "z", gomock.Any()),
						)
						result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})

						Expect(err).NotTo(HaveOccurred())
						Expect(result.RequeueAfter).To(BeZero())

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
						mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
						mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
					)
					result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
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
						mockCVClientBuilder.EXPECT().New(gomock.Any()).Return(mockCVClient),
						mockCVClient.EXPECT().GetClusterVersion().Return(testClusterVersion, nil),
						mockConfigManagerBuilder.EXPECT().New(gomock.Any(), gomock.Any()).Return(mockConfigManager),
						mockConfigManager.EXPECT().Into(gomock.Any()).SetArg(0, cfg),
						mockClusterUpgraderBuilder.EXPECT().NewClient(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), upgradeConfig.Spec.Type).Return(mockClusterUpgrader, nil),
					)
					result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})
		})
	})
})

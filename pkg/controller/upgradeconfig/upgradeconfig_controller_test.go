package upgradeconfig

import (
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"
	"github.com/onsi/gomega/gstruct"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	mockUpgrader "github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader/mocks"
	mockMaintenance "github.com/openshift/managed-upgrade-operator/pkg/maintenance/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	k8serrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeConfigController", func() {
	var (
		upgradeConfigName      types.NamespacedName
		upgradeConfig          *upgradev1alpha1.UpgradeConfig
		reconciler             *ReconcileUpgradeConfig
		mockKubeClient         *mocks.MockClient
		mockUpdater            *mocks.MockStatusWriter
		mockMaintenanceBuilder *mockMaintenance.MockMaintenanceBuilder
		mockClusterUpgrader    *mockUpgrader.MockClusterUpgrader
		mockCtrl               *gomock.Controller
		testScheme             *runtime.Scheme
	)

	BeforeEach(func() {
		var err error
		testScheme, err = buildScheme()
		Expect(err).NotTo(HaveOccurred())

		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockUpdater = mocks.NewMockStatusWriter(mockCtrl)
		mockMaintenanceBuilder = mockMaintenance.NewMockMaintenanceBuilder(mockCtrl)
		mockClusterUpgrader = mockUpgrader.NewMockClusterUpgrader(mockCtrl)
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		clusterUpgraderBuilder := func() cluster_upgrader.ClusterUpgrader {
			return mockClusterUpgrader
		}
		reconciler = &ReconcileUpgradeConfig{
			mockKubeClient,
			testScheme,
			mockMaintenanceBuilder,
			clusterUpgraderBuilder,
		}
	})

	Context("Reconcile", func() {

		Context("When an UpgradeConfig doesn't exist", func() {
			JustBeforeEach(func() {
				notFound := k8serrs.NewNotFound(schema.GroupResource{}, upgradeConfigName.Name)
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Return(notFound)
			})

			It("Returns without error", func() {
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("When fetching an UpgradeConfig fails", func() {
			var fakeError error
			JustBeforeEach(func() {
				fakeError = k8serrs.NewInternalError(fmt.Errorf("a fake error"))
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1).Return(fakeError)
			})
			It("Requeues the request", func() {
				result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
				Expect(err).To(Equal(fakeError))
				// This doesn't make a great deal of sense to me, but it's what the
				// boilerplate operator code says/does
				Expect(result.Requeue).To(BeFalse())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("When an UpgradeConfig exists", func() {
			JustBeforeEach(func() {
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
			})

			Context("When getting a clusterversion fails", func() {
				var fakeError = fmt.Errorf("error getting clusterversion")
				JustBeforeEach(func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).Times(1).Return(fakeError)
				})
				It("Does not proceed and returns the error", func() {
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).To(Equal(fakeError))
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			Context("When a cluster is upgrading", func() {
				JustBeforeEach(func() {
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							DesiredUpdate: &configv1.Update{
								Version: "not the same version",
							},
						},
						Status: configv1.ClusterVersionStatus{
							Conditions: []configv1.ClusterOperatorStatusCondition{
								{
									Type:   configv1.OperatorProgressing,
									Status: configv1.ConditionTrue,
								},
							},
						},
					}).Times(1)
				})

				It("Returns an empty result and no error", func() {
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
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
					mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, configv1.ClusterVersion{}).Times(1)
				})
				Context("When updating the UpdateConfig's history fails", func() {
					It("Returns an error", func() {
						fakeError := k8serrs.NewInternalError(fmt.Errorf("a fake error"))
						mockKubeClient.EXPECT().Status().Return(mockUpdater)
						mockUpdater.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fakeError)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When the history is added to the UpgradeConfig", func() {
					It("Adds it successfully", func() {
						matcher := testStructs.NewUpgradeConfigMatcher()
						mockKubeClient.EXPECT().Status().Return(mockUpdater).AnyTimes()
						mockUpdater.EXPECT().Update(gomock.Any(), matcher).AnyTimes()
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
						mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any())
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
				mockKubeClient.EXPECT().Get(gomock.Any(), upgradeConfigName, gomock.Any()).SetArg(2, *upgradeConfig).Times(1)
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, configv1.ClusterVersion{}).Times(1)
			})

			Context("When the upgrade phase is New", func() {
				JustBeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseNew
				})

				Context("When the cluster is not ready to upgrade", func() {
					// TODO: This is never true at the moment - no readiness check implemented
				})

				Context("When the cluster is ready to upgrade", func() {
					It("Invokes the upgrader", func() {
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
						mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any())
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})
			})

			Context("When the upgrade phase is Pending", func() {
				JustBeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhasePending
				})

				Context("When the cluster is ready to upgrade", func() {
					Context("When a maintenance client can't be built", func() {
						var fakeError = fmt.Errorf("a maintenance builder error")
						It("does not proceed with upgrading the cluster", func() {
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
							mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any()).Return(nil, fakeError)
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(Equal(fakeError))
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})

					Context("When a maintenance client can be built", func() {
						It("Invokes the upgrader", func() {
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
							mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any())
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})

					Context("When invoking the upgrader fails", func() {
						var fakeError = fmt.Errorf("the upgrader failed")
						It("reacts accordingly", func() {
							// All it does here is log at the moment
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(fakeError)
							mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any())
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).NotTo(HaveOccurred())
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})
				})

				Context("When the cluster is not ready to upgrade", func() {
					// TODO: This is never true at the moment - no readiness check implemented
				})
			})

			Context("When the upgrade phase is Upgrading", func() {
				JustBeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUpgrading
				})

				Context("When a maintenance client can't be built", func() {
					var fakeError = fmt.Errorf("a maintenance builder error")
					It("does not proceed with upgrading the cluster", func() {
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any()).Return(nil, fakeError)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When a maintenance client can be built", func() {
					It("proceeds with upgrading the cluster", func() {
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
						mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any())
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When invoking the upgrader fails", func() {
					var fakeError = fmt.Errorf("the upgrader failed")
					It("reacts accordingly", func() {
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(fakeError)
						mockMaintenanceBuilder.EXPECT().NewClient(gomock.Any())
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).NotTo(HaveOccurred())
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})
			})

			Context("When the upgrade phase is Upgraded", func() {
				JustBeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUpgraded
				})
				It("does nothing", func() {
					mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			Context("When the upgrade phase is Failed", func() {
				JustBeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseFailed
				})
				It("does nothing", func() {
					mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
					result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeFalse())
					Expect(result.RequeueAfter).To(BeZero())
				})
			})

			Context("When the upgrade phase is Unknown", func() {
				JustBeforeEach(func() {
					upgradeConfig.Status.History[0].Phase = upgradev1alpha1.UpgradePhaseUnknown
				})
				It("does nothing", func() {
					mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
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

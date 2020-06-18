package upgradeconfig

import (
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega/gstruct"
	"github.com/openshift/managed-upgrade-operator/pkg/maintenance"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/event"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"

	k8serrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UpgradeConfigController", func() {
	var (
		upgradeConfigName     types.NamespacedName
		upgradeConfig         *upgradev1alpha1.UpgradeConfig
		reconciler            *ReconcileUpgradeConfig
		mockKubeClient        *mocks.MockClient
		mockUpdater           *mocks.MockStatusWriter
		mockMaintenanceClient *mocks.MockMaintenance
		mockClusterUpgrader   *mocks.MockClusterUpgrader
		mockCtrl              *gomock.Controller
		testScheme            *runtime.Scheme
	)

	BeforeEach(func() {
		testScheme = runtime.NewScheme()
		if err := configv1.Install(testScheme); err != nil {
			log.Error(err, "Unable to add config version to scheme: %s", err)
			os.Exit(1)
		}
		if err := routev1.Install(testScheme); err != nil {
			log.Error(err, "Unable to add route version to scheme: %s", err)
			os.Exit(1)
		}
		if err := machineapi.AddToScheme(testScheme); err != nil {
			log.Error(err, "Unable to add machineapi version to scheme: %s", err)
			os.Exit(1)
		}
		if err := machineconfigapi.Install(testScheme); err != nil {
			log.Error(err, "Unable to add machineconfigapi version to scheme: %s", err)
			os.Exit(1)
		}
		_ = upgradev1alpha1.SchemeBuilder.AddToScheme(testScheme)

		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockUpdater = mocks.NewMockStatusWriter(mockCtrl)
		mockMaintenanceClient = mocks.NewMockMaintenance(mockCtrl)
		mockClusterUpgrader = mocks.NewMockClusterUpgrader(mockCtrl)
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
		maintenanceBuilder := func(client client.Client) (maintenance.Maintenance, error) {
			return mockMaintenanceClient, nil
		}
		clusterUpgraderBuilder := func() ClusterUpgrader {
			return mockClusterUpgrader
		}
		reconciler = &ReconcileUpgradeConfig{
			mockKubeClient,
			testScheme,
			maintenanceBuilder,
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
						JustBeforeEach(func() {
							maintenanceBuilder := func(client client.Client) (maintenance.Maintenance, error) {
								return mockMaintenanceClient, fakeError
							}
							reconciler.maintenanceClientBuilder = maintenanceBuilder
						})
						It("does not proceed with upgrading the cluster", func() {
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
							result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
							Expect(err).To(Equal(fakeError))
							Expect(result.Requeue).To(BeFalse())
							Expect(result.RequeueAfter).To(BeZero())
						})
					})

					Context("When a maintenance client can be built", func() {
						It("Invokes the upgrader", func() {
							mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
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
					JustBeforeEach(func() {
						maintenanceBuilder := func(client client.Client) (maintenance.Maintenance, error) {
							return mockMaintenanceClient, fakeError
						}
						reconciler.maintenanceClientBuilder = maintenanceBuilder
					})
					It("does not proceed with upgrading the cluster", func() {
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
						result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: upgradeConfigName})
						Expect(err).To(Equal(fakeError))
						Expect(result.Requeue).To(BeFalse())
						Expect(result.RequeueAfter).To(BeZero())
					})
				})

				Context("When a maintenance client can be built", func() {
					It("proceeds with upgrading the cluster", func() {
						mockClusterUpgrader.EXPECT().UpgradeCluster(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)
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

	Context("Update", func() {
		var scp StatusChangedPredicate
		Context("When the old object meta doesn't exist", func() {
			It("will not return true", func() {
				result := scp.Update(event.UpdateEvent{MetaOld: nil, ObjectOld: upgradeConfig, MetaNew: upgradeConfig.GetObjectMeta(), ObjectNew: upgradeConfig})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the old object doesn't exist", func() {
			It("will not return true", func() {
				result := scp.Update(event.UpdateEvent{MetaOld: upgradeConfig.GetObjectMeta(), ObjectOld: nil, MetaNew: upgradeConfig.GetObjectMeta(), ObjectNew: upgradeConfig})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the new object meta doesn't exist", func() {
			It("will not return true", func() {
				result := scp.Update(event.UpdateEvent{MetaOld: upgradeConfig.GetObjectMeta(), ObjectOld: upgradeConfig, MetaNew: nil, ObjectNew: upgradeConfig})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the new object doesn't exist", func() {
			It("will not return true", func() {
				result := scp.Update(event.UpdateEvent{MetaOld: upgradeConfig.GetObjectMeta(), ObjectOld: upgradeConfig, MetaNew: upgradeConfig.GetObjectMeta(), ObjectNew: nil})
				Expect(result).To(BeFalse())
			})
		})
		Context("When the old and new events match", func() {
			It("will return true", func() {
				uc1 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				uc2 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				result := scp.Update(event.UpdateEvent{MetaOld: uc1.GetObjectMeta(), ObjectOld: uc1, MetaNew: uc2.GetObjectMeta(), ObjectNew: uc2})
				Expect(result).To(BeTrue())
			})
		})
		Context("When the old and new events do not match", func() {
			It("will not return true", func() {
				uc1 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				uc2 := testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
				uc2.Status.History = []upgradev1alpha1.UpgradeHistory{{Version: "something else"}}
				result := scp.Update(event.UpdateEvent{MetaOld: uc1.GetObjectMeta(), ObjectOld: uc1, MetaNew: uc2.GetObjectMeta(), ObjectNew: uc2})
				Expect(result).To(BeFalse())
			})
		})

	})
})

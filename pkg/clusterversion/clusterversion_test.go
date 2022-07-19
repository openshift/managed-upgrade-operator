package clusterversion

import (
	"context"
	"fmt"
	"reflect"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
)

var _ = Describe("ClusterVersion client and utils", func() {

	var (
		cvClient          ClusterVersion
		mockCtrl          *gomock.Controller
		mockKubeClient    *mocks.MockClient
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		upgradeConfigName types.NamespacedName
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		cvClient = &clusterVersionClient{mockKubeClient}
		upgradeConfigName = types.NamespacedName{
			Name:      "test-upgradeconfig",
			Namespace: "test-namespace",
		}
		upgradeConfig = testStructs.NewUpgradeConfigBuilder().WithNamespacedName(upgradeConfigName).GetUpgradeConfig()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("ClusterVersion client", func() {
		It("should get the ClusterVersion resource", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{
					ObjectMeta: metav1.ObjectMeta{Name: OSD_CV_NAME},
				}).Return(nil),
			)
			clusterVersion, err := cvClient.GetClusterVersion()
			Expect(clusterVersion).To(Not(BeNil()))
			Expect(clusterVersion.Name).To(Equal(OSD_CV_NAME))
			Expect(err).Should(BeNil())
		})

		It("should error if ClusterVersion resource is not found", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.NewNotFound(schema.GroupResource{
					Group: configv1.GroupName, Resource: "ClusterVersion"}, OSD_CV_NAME),
				),
			)
			clusterVersion, err := cvClient.GetClusterVersion()
			Expect(clusterVersion).To(BeNil())
			Expect(err).Should(Not(BeNil()))
		})

		Context("When the cluster's desired version matches the UpgradeConfig's", func() {
			It("Indicates the upgrade has commenced", func() {
				gomock.InOrder(
					mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel:       upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: &configv1.Update{Version: upgradeConfig.Spec.Desired.Version},
						},
					}).Return(nil),
				)
				hasCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasCommenced).To(BeTrue())
			})
		})

		Context("When setting the ClusterVersions version", func() {
			Context("When the cluster is not on the same channel as the UpgradeConfig", func() {
				It("Updates the cluster's update channel", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel:       upgradeConfig.Spec.Desired.Channel + "not-the-same",
							DesiredUpdate: nil,
						},
					}
					updatedClusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel:       upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: nil,
						},
						Status: configv1.ClusterVersionStatus{
							AvailableUpdates: []configv1.Release{
								{
									Version: upgradeConfig.Spec.Desired.Version,
									Image:   "quay.io/this-doesnt-exist",
								},
							},
						},
					}
					channelPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"channel":"%s"}}`, upgradeConfig.Spec.Desired.Channel)))
					versionPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"version":"%s","image":null}}}`, upgradeConfig.Spec.Desired.Version)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch) error {
								Expect(reflect.DeepEqual(p, channelPatch)).To(BeTrue())
								return nil
							}),
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, updatedClusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(isCompleted).To(BeTrue())
				})
			})

			Context("When the cluster's desired version is missing", func() {
				It("Sets the desired version to that of the UpgradeConfig's", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel:       upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: nil,
						},
						Status: configv1.ClusterVersionStatus{
							AvailableUpdates: []configv1.Release{
								{
									Version: upgradeConfig.Spec.Desired.Version,
									Image:   "quay.io/dummy-image-for-test",
								},
							},
						},
					}
					versionPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"version":"%s","image":null}}}`, upgradeConfig.Spec.Desired.Version)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(isCompleted).To(BeTrue())
				})
			})

			Context("When the cluster's desired version does not match the UpgradeConfig's", func() {
				It("Sets the desired version to that of the UpgradeConfig's", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel: upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: &configv1.Update{
								Version: "something different",
							},
						},
						Status: configv1.ClusterVersionStatus{
							AvailableUpdates: []configv1.Release{
								{
									Version: upgradeConfig.Spec.Desired.Version,
									Image:   "quay.io/dummy-image-for-test",
								},
							},
						},
					}
					versionPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"version":"%s","image":null}}}`, upgradeConfig.Spec.Desired.Version)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(isCompleted).To(BeTrue())
				})
			})
		})

		Context("When checking ClusterOperators", func() {
			Context("When ClusterOperators are not degraded", func() {
				var operatorList configv1.ClusterOperatorList

				JustBeforeEach(func() {
					operatorList = configv1.ClusterOperatorList{
						Items: []configv1.ClusterOperator{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "operator1",
								},
								Status: configv1.ClusterOperatorStatus{
									Conditions: []configv1.ClusterOperatorStatusCondition{
										{Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue},
									},
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "operator2",
								},
								Status: configv1.ClusterOperatorStatus{
									Conditions: []configv1.ClusterOperatorStatusCondition{
										{Type: configv1.OperatorDegraded, Status: configv1.ConditionFalse},
									},
								},
							},
						},
					}
				})

				It("will indicate that no ClusterOperators are degraded", func() {
					gomock.InOrder(
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, operatorList),
					)
					result, err := cvClient.HasDegradedOperators()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(result.Degraded)).To(BeZero())
				})
			})

			Context("When operators are degraded", func() {
				var operatorList *configv1.ClusterOperatorList

				JustBeforeEach(func() {
					operatorList = &configv1.ClusterOperatorList{
						Items: []configv1.ClusterOperator{
							{
								ObjectMeta: metav1.ObjectMeta{Name: "I'm a broken operator"},
								Spec:       configv1.ClusterOperatorSpec{},
								Status: configv1.ClusterOperatorStatus{
									Conditions: []configv1.ClusterOperatorStatusCondition{
										{Type: configv1.OperatorDegraded, Status: configv1.ConditionTrue},
									},
								},
							},
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "I'm an unavailable operator",
								},
								Spec: configv1.ClusterOperatorSpec{},
								Status: configv1.ClusterOperatorStatus{
									Conditions: []configv1.ClusterOperatorStatusCondition{
										{Type: configv1.OperatorAvailable, Status: configv1.ConditionFalse},
									},
								},
							},
						},
					}
				})
				It("will indicate that ClusterOperators are degraded", func() {
					gomock.InOrder(
						mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *operatorList),
					)
					result, err := cvClient.HasDegradedOperators()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(result.Degraded)).To(Equal(2))
				})
			})
		})
		Context("When setting the ClusterVersion image", func() {
			Context("When the clusterversion desired image is missing", func() {
				It("Sets the desired image from the value of upgradeconfig", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel: upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: &configv1.Update{
								Version: "Some version",
							},
						},
					}
					upgradeConfig.Spec.Desired.Image = "quay.io/test/test-image"
					updatePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"image":"%s","version":null}}}`, upgradeConfig.Spec.Desired.Image)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch) error {
								Expect(reflect.DeepEqual(p, updatePatch)).To(BeTrue())
								return nil
							}),
					)
					result, err := cvClient.EnsureDesiredConfig(upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeTrue())
				})
			})
			Context("When the clusterversion desired image does not match the upgradeconfig", func() {
				It("Sets the desired image from the value of upgradeconfig", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel: upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: &configv1.Update{
								Image: "quay.io/test/test-image2",
							},
						},
					}
					upgradeConfig.Spec.Desired.Image = "quay.io/test/test-image"
					updatePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"image":"%s","version":null}}}`, upgradeConfig.Spec.Desired.Image)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch) error {
								Expect(reflect.DeepEqual(p, updatePatch)).To(BeTrue())
								return nil
							}),
					)
					result, err := cvClient.EnsureDesiredConfig(upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(BeTrue())
				})
			})
			Context("When the clusterversion desired image matches the upgradeconfig", func() {
				It("Indicates that the cluster is upgraded or upgrading", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel: upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: &configv1.Update{
								Image: "quay.io/test/test-image",
							},
						},
					}
					upgradeConfig.Spec.Desired.Image = "quay.io/test/test-image"
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
					)
					hasCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(hasCommenced).To(BeTrue())
				})
			})
		})
	})

})

package clusterversion

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	"go.uber.org/mock/gomock"
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
			clusterVersion, err := cvClient.GetClusterVersion(context.Background())
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
			clusterVersion, err := cvClient.GetClusterVersion(context.Background())
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
				hasCommenced, err := cvClient.HasUpgradeCommenced(context.Background(), upgradeConfig)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasCommenced).To(BeTrue())
			})
		})

		Context("When setting the ClusterVersions version", func() {
			Context("When the version is conditional", func() {
				It("Updates the cluster's update image", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel:       upgradeConfig.Spec.Desired.Channel,
							DesiredUpdate: nil,
						},
						Status: configv1.ClusterVersionStatus{
							ConditionalUpdates: []configv1.ConditionalUpdate{
								{
									Release: configv1.Release{
										Version: upgradeConfig.Spec.Desired.Version,
										Image:   "quay.io/this-doesnt-exist",
									},
								},
							},
						},
					}

					versionPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"version":"%s","image":"%s"}}}`, upgradeConfig.Spec.Desired.Version, clusterVersion.Status.ConditionalUpdates[0].Release.Image)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(isCompleted).To(BeTrue())
				})
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
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, channelPatch)).To(BeTrue())
								return nil
							}),
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, updatedClusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
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
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
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
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, versionPatch)).To(BeTrue())
								return nil
							}),
					)
					isCompleted, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
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
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, updatePatch)).To(BeTrue())
								return nil
							}),
					)
					result, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
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
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(reflect.DeepEqual(p, updatePatch)).To(BeTrue())
								return nil
							}),
					)
					result, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
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
					hasCommenced, err := cvClient.HasUpgradeCommenced(context.Background(), upgradeConfig)
					Expect(err).NotTo(HaveOccurred())
					Expect(hasCommenced).To(BeTrue())
				})
			})
			Context("When the clusterversion channel differs from the upgradeconfig and image is set", func() {
				It("Updates the channel before setting the desired image", func() {
					clusterVersion := configv1.ClusterVersion{
						Spec: configv1.ClusterVersionSpec{
							Channel: "stable-4.20",
							DesiredUpdate: &configv1.Update{
								Version: "Some version",
							},
						},
					}
					upgradeConfig.Spec.Desired.Channel = "stable-4.21"
					upgradeConfig.Spec.Desired.Image = "quay.io/test/test-image"
					channelPatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"channel":"%s"}}`, upgradeConfig.Spec.Desired.Channel)))
					imagePatch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf(`{"spec":{"desiredUpdate":{"image":"%s","version":null}}}`, upgradeConfig.Spec.Desired.Image)))
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(p).To(Equal(channelPatch), "the first patch must change only the channel before requesting the image")
								return nil
							}),
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, clusterVersion).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, po ...client.PatchOption) error {
								Expect(p).To(Equal(imagePatch), "after refreshing the channel, the second patch must set the image and clear the version")
								return nil
							}),
					)
					result, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
					Expect(err).NotTo(HaveOccurred(), "updating the channel followed by the desired image should succeed")
					Expect(result).To(BeTrue(), "a successful channel and image update should report the upgrade triggered")
				})
			})
		})
	})

	for _, source := range []string{UpgradeWithImage, UpgradeWithChannelVersion} {
		Context("EnsureDesiredConfig using "+source, func() {
			var initial, refreshed configv1.ClusterVersion
			var updatePatch client.Patch

			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Channel = "stable-4.21"
				upgradeConfig.Spec.Desired.Version = "4.21.1"
				upgradeConfig.Spec.Desired.Image = ""
				updatePatch = client.RawPatch(types.MergePatchType, []byte(`{"spec":{"desiredUpdate":{"version":"4.21.1","image":null}}}`))
				if source == UpgradeWithImage {
					upgradeConfig.Spec.Desired.Image = "quay.io/test/test-image"
					updatePatch = client.RawPatch(types.MergePatchType, []byte(`{"spec":{"desiredUpdate":{"image":"quay.io/test/test-image","version":null}}}`))
				}
				initial = configv1.ClusterVersion{
					ObjectMeta: metav1.ObjectMeta{Name: OSD_CV_NAME, ResourceVersion: "1"},
					Spec:       configv1.ClusterVersionSpec{Channel: "stable-4.20"},
				}
				refreshed = *initial.DeepCopy()
				refreshed.ResourceVersion = "2"
				refreshed.Spec.Channel = upgradeConfig.Spec.Desired.Channel
				refreshed.Status.AvailableUpdates = []configv1.Release{{
					Version: upgradeConfig.Spec.Desired.Version,
					Image:   "quay.io/test/test-image",
				}}
			})

			for _, tc := range []struct {
				name, channel string
			}{
				{"quotes", `stable-"4.21"`},
				{"backslashes", `stable-\4.21\`},
				{"newline", "stable-4.21\nnext-line"},
				{"JSON field injection", `stable-4.21","desiredUpdate":{"image":"injected"},"upstream":"https://attacker.invalid`},
			} {
				It("preserves channel strings containing "+tc.name+" without patching other fields", func() {
					upgradeConfig.Spec.Desired.Channel = tc.channel
					refreshed.Spec.Channel = tc.channel
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: OSD_CV_NAME}, gomock.Any()).SetArg(2, initial).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, _ ...client.PatchOption) error {
								Expect(p.Type()).To(Equal(types.MergePatchType), "the channel update must remain a JSON merge patch")
								data, err := p.Data(cv)
								Expect(err).NotTo(HaveOccurred(), "channel patch bytes should be available for %s", tc.name)
								var decoded map[string]interface{}
								Expect(json.Unmarshal(data, &decoded)).To(Succeed(), "channel patch must be valid JSON for %s: %s", tc.name, data)
								Expect(decoded).To(Equal(map[string]interface{}{
									"spec": map[string]interface{}{"channel": tc.channel},
								}), "the patch must preserve the exact channel string and contain no injected or unrelated fields")
								return nil
							}),
						mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: OSD_CV_NAME}, gomock.Any()).SetArg(2, refreshed).Return(nil),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, cv *configv1.ClusterVersion, p client.Patch, _ ...client.PatchOption) error {
								Expect(cv.ResourceVersion).To(Equal(refreshed.ResourceVersion), "the desired update must use the ClusterVersion fetched after the channel patch")
								Expect(p).To(Equal(updatePatch), "only after patching and refreshing the channel should the desired image or version be set")
								return nil
							}),
					)
					triggered, err := cvClient.EnsureDesiredConfig(context.Background(), upgradeConfig)
					Expect(err).NotTo(HaveOccurred(), "encoding %s in the channel must not break the upgrade request", tc.name)
					Expect(triggered).To(BeTrue(), "both ordered patches should complete for channels containing %s", tc.name)
				})
			}

			for _, earlierParent := range []bool{false, true} {
				It(fmt.Sprintf("shares one bounded deadline across GET/PATCH/GET/PATCH (earlier parent: %t)", earlierParent), func() {
					parent := context.Background()
					var parentDeadline time.Time
					if earlierParent {
						parentDeadline = time.Now().Add(10 * time.Second)
						var cancel context.CancelFunc
						parent, cancel = context.WithDeadline(parent, parentDeadline)
						defer cancel()
					}
					started := time.Now()
					var deadline time.Time
					var contexts []context.Context
					observe := func(ctx context.Context, stage string) {
						got, ok := ctx.Deadline()
						Expect(ok).To(BeTrue(), "%s must have a bounded API deadline", stage)
						Expect(ctx.Err()).NotTo(HaveOccurred(), "%s must receive a live context, not the canceled context from the preceding GET", stage)
						if len(contexts) == 0 {
							deadline = got
							if earlierParent {
								Expect(got).To(Equal(parentDeadline), "the initial GET must respect the earlier parent deadline")
							} else {
								Expect(got.Before(started.Add(clusterVersionAPITimeout))).To(BeFalse(), "the initial GET should receive the full aggregate API budget")
								Expect(got.After(time.Now().Add(clusterVersionAPITimeout))).To(BeFalse(), "the API deadline must not exceed the configured timeout")
							}
						}
						Expect(got).To(Equal(deadline), "%s must share the initial deadline rather than restart the API timeout", stage)
						contexts = append(contexts, ctx)
					}
					gomock.InOrder(
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, _ types.NamespacedName, cv *configv1.ClusterVersion, _ ...client.GetOption) error {
								observe(ctx, "initial GET")
								*cv = initial
								return nil
							}),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
								observe(ctx, "channel PATCH")
								return nil
							}),
						mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, _ types.NamespacedName, cv *configv1.ClusterVersion, _ ...client.GetOption) error {
								observe(ctx, "refresh GET")
								*cv = refreshed
								return nil
							}),
						mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
								observe(ctx, "final PATCH")
								return nil
							}),
					)
					triggered, err := cvClient.EnsureDesiredConfig(parent, upgradeConfig)
					Expect(err).NotTo(HaveOccurred(), "all four API operations should succeed within the shared deadline")
					Expect(triggered).To(BeTrue(), "the bounded request should complete the desired update")
					Expect(contexts).To(HaveLen(4), "each API operation must be checked for the aggregate deadline")
					for i, ctx := range contexts {
						Expect(ctx.Err()).To(MatchError(context.Canceled), "API context %d should be released when the request returns", i)
					}
					Expect(parent.Err()).NotTo(HaveOccurred(), "releasing API contexts must not cancel the caller's context")
				})
			}

			for _, tc := range []struct {
				stage  int
				cancel bool
			}{
				{1, true}, {2, true},
				{0, false}, {1, false}, {2, false}, {3, false},
			} {
				stages := []string{"initial GET", "channel PATCH", "refresh GET", "final PATCH"}
				It(fmt.Sprintf("stops at %s failure without subsequent writes (cancellation: %t)", stages[tc.stage], tc.cancel), func() {
					parent, cancel := context.WithCancel(context.Background())
					defer cancel()
					var wantErr error = errors.NewServiceUnavailable("ClusterVersion API unavailable")
					if tc.cancel {
						wantErr = context.Canceled
					}
					fail := func(ctx context.Context) error {
						if tc.cancel {
							cancel()
							Expect(ctx.Done()).To(BeClosed(), "%s must observe cancellation of the actual parent context", stages[tc.stage])
							Expect(ctx.Err()).To(MatchError(context.Canceled), "%s must receive the caller's cancellation", stages[tc.stage])
							return ctx.Err()
						}
						return wantErr
					}
					// Register only calls through the failure; any later read or write is unexpected.
					var calls []interface{}
					for stage := 0; stage <= tc.stage; stage++ {
						if stage == 0 || stage == 2 {
							calls = append(calls, mockKubeClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
								func(ctx context.Context, _ types.NamespacedName, cv *configv1.ClusterVersion, _ ...client.GetOption) error {
									if stage == tc.stage {
										return fail(ctx)
									}
									*cv = initial
									if stage == 2 {
										*cv = refreshed
									}
									return nil
								}))
						} else {
							calls = append(calls, mockKubeClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
								func(ctx context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
									if stage == tc.stage {
										return fail(ctx)
									}
									return nil
								}))
						}
					}
					gomock.InOrder(calls...)
					triggered, err := cvClient.EnsureDesiredConfig(parent, upgradeConfig)
					Expect(err).To(MatchError(wantErr), "%s failure must be returned to the caller", stages[tc.stage])
					Expect(triggered).To(BeFalse(), "%s failure must not report a triggered upgrade", stages[tc.stage])
				})
			}
		})
	}

	Describe("GetPrecedingVersion", func() {
		var (
			clusterVersion configv1.ClusterVersion
		)

		BeforeEach(func() {
			clusterVersion = configv1.ClusterVersion{
				Status: configv1.ClusterVersionStatus{
					Desired: configv1.Release{
						Version: "cv-desired-version",
					},
				},
			}

		})

		Context("When CV desired version is different from upgradeConfig desired version", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = "uc-desired-version"
			})

			It("Returns the CV version", func() {
				precedingVersion := GetPrecedingVersion(&clusterVersion, upgradeConfig)
				Expect(precedingVersion).To(Equal(clusterVersion.Status.Desired.Version))
			})
		})

		Context("When CV desired version is equal to upgradeconfig desired version", func() {
			BeforeEach(func() {
				upgradeConfig.Spec.Desired.Version = "cv-desired-version"
			})
			Context("When History is empty", func() {
				It("Returns the CV version", func() {
					precedingVersion := GetPrecedingVersion(&clusterVersion, upgradeConfig)
					Expect(precedingVersion).To(Equal(clusterVersion.Status.Desired.Version))
				})
			})

			Context("When History is not empty", func() {
				BeforeEach(func() {
					clusterVersion.Status.History = []configv1.UpdateHistory{
						{
							State:   configv1.PartialUpdate,
							Version: "partial-updated-version-1",
						},
						{
							State:   configv1.CompletedUpdate,
							Version: "cv-desired-version",
						},
						{
							State:   configv1.CompletedUpdate,
							Version: "completely-updated-version-1",
						},
						{
							State:   configv1.CompletedUpdate,
							Version: "completely-updated-version-2",
						},
					}
				})

				It("Returns the first different version from history in status CompletedUpdate", func() {
					precedingVersion := GetPrecedingVersion(&clusterVersion, upgradeConfig)
					Expect(precedingVersion).To(Equal(clusterVersion.Status.History[2].Version))
				})

				Context("When History has no CompletedUpdate", func() {
					BeforeEach(func() {
						clusterVersion.Status.History = []configv1.UpdateHistory{
							{
								State:   configv1.PartialUpdate,
								Version: "partial-updated-version-1",
							},
						}
					})

					It("Returns the CV version", func() {
						precedingVersion := GetPrecedingVersion(&clusterVersion, upgradeConfig)
						Expect(precedingVersion).To(Equal(clusterVersion.Status.Desired.Version))
					})
				})
			})

		})

	})

})

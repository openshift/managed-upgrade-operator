package clusterversion

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Architecture migration", func() {
	var uc *upgradev1alpha1.UpgradeConfig

	BeforeEach(func() {
		uc = &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{
			Desired: upgradev1alpha1.Update{Version: "4.18.1", Architecture: configv1.ClusterVersionArchitectureMulti},
		}}
	})

	DescribeTable("Submitting a migration request", func(initial *configv1.Update) {
		scheme := runtime.NewScheme()
		Expect(configv1.AddToScheme(scheme)).To(Succeed(), "register the ClusterVersion API")
		cv := &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: OSD_CV_NAME},
			Spec:       configv1.ClusterVersionSpec{Channel: "stable-4.18", DesiredUpdate: initial.DeepCopy()},
			Status:     configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1", Image: "single-arch-image"}, History: []configv1.UpdateHistory{{Version: "4.18.1", Image: "single-arch-image", State: configv1.CompletedUpdate}}},
		}
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cv).Build()
		c := NewCVClient(kube)
		ctx := context.Background()
		key := types.NamespacedName{Name: OSD_CV_NAME}

		By("not recognizing the old single-architecture payload as the migration request")
		started, err := c.HasUpgradeCommenced(ctx, uc)
		Expect(err).NotTo(HaveOccurred(), "check whether migration has commenced")
		Expect(started).To(BeFalse(), "migration must not commence before submitting its request")

		By("submitting the migration idempotently and preserving the channel")
		for i := 0; i < 2; i++ {
			done, err := c.EnsureDesiredConfig(ctx, uc)
			Expect(err).NotTo(HaveOccurred(), "submit migration on reconciliation %d", i+1)
			Expect(done).To(BeTrue(), "migration request should be applied on reconciliation %d", i+1)
		}
		Expect(kube.Get(ctx, key, cv)).To(Succeed(), "read ClusterVersion after submitting migration")
		Expect(cv.Spec.DesiredUpdate).To(Equal(&configv1.Update{
			Version: "4.18.1", Architecture: configv1.ClusterVersionArchitectureMulti,
		}), "request Multi at the current version without an image or force flag")
		Expect(cv.Spec.Channel).To(Equal("stable-4.18"), "migration must preserve the cluster channel")
		started, err = c.HasUpgradeCommenced(ctx, uc)
		Expect(err).NotTo(HaveOccurred(), "check the submitted migration request")
		Expect(started).To(BeTrue(), "recognize the submitted migration request")

		By("rejecting a version change during a single-to-Multi migration")
		beforeVersionChange := cv.Spec.DeepCopy()
		uc.Spec.Desired.Version = "4.18.2"
		done, err := c.EnsureDesiredConfig(ctx, uc)
		Expect(err).To(HaveOccurred(), "reject the migration with a different version")
		Expect(done).To(BeFalse(), "the version change must not be applied")
		started, err = c.HasUpgradeCommenced(ctx, uc)
		Expect(err).To(HaveOccurred(), "reject a different version when checking migration commencement")
		Expect(started).To(BeFalse(), "the rejected version change must not be recognized as commenced")
		Expect(kube.Get(ctx, key, cv)).To(Succeed(), "read ClusterVersion after rejecting the version change")
		Expect(&cv.Spec).To(Equal(beforeVersionChange), "a rejected version change must not modify ClusterVersion")
		uc.Spec.Desired.Version = "4.18.1"

		By("rejecting a channel change even when the version and architecture match")
		before := cv.Spec.DeepCopy()
		uc.Spec.Desired.Channel = "fast-4.18"
		started, err = c.HasUpgradeCommenced(ctx, uc)
		Expect(err).To(HaveOccurred(), "reject the changed channel when checking migration commencement")
		Expect(started).To(BeFalse(), "the rejected channel change must not be recognized as commenced")
		done, err = c.EnsureDesiredConfig(ctx, uc)
		Expect(err).To(HaveOccurred(), "reject the migration with a changed channel")
		Expect(done).To(BeFalse(), "the channel change must not be applied")
		Expect(kube.Get(ctx, key, cv)).To(Succeed(), "read ClusterVersion after rejecting the channel change")
		Expect(&cv.Spec).To(Equal(before), "a rejected channel change must not modify ClusterVersion")
	},
		Entry("without an existing desired update", (*configv1.Update)(nil)),
		Entry("replacing an existing single-architecture request", &configv1.Update{Version: "4.18.1", Image: "single-arch-image", Force: true}),
	)

	Context("Checking migration completion", func() {
		old := configv1.UpdateHistory{Version: "4.18.1", Image: "single", State: configv1.CompletedUpdate}

		DescribeTable("Recognizing the completed Multi payload", func(architecture configv1.ClusterVersionArchitecture, history []configv1.UpdateHistory, completed bool) {
			cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{Version: "4.18.1", Image: "multi", Architecture: architecture}, History: history,
			}}
			Expect((&clusterVersionClient{}).HasUpgradeCompleted(cv, uc)).To(Equal(completed),
				"completion must reflect the Multi payload and its update history")
		},
			Entry("does not accept the old payload", configv1.ClusterVersionArchitecture(""), []configv1.UpdateHistory{old}, false),
			Entry("waits when status is updated before history", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory{old}, false),
			Entry("waits when there is no history", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory(nil), false),
			Entry("waits while migration is in progress", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory{{Version: "4.18.1", Image: "multi", State: configv1.PartialUpdate}, old}, false),
			Entry("recognizes a completed migration", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory{{Version: "4.18.1", Image: "multi", State: configv1.CompletedUpdate}, old}, true),
		)

		It("waits for the requested version's payload to complete", func() {
			uc.Spec.Desired.Version = "4.18.2"
			cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{Version: "4.18.1", Architecture: "Multi", Image: "multi-current"},
				History: []configv1.UpdateHistory{{Version: "4.18.1", Image: "multi-current", State: configv1.CompletedUpdate}},
			}}
			c := &clusterVersionClient{}
			Expect(c.HasUpgradeCompleted(cv, uc)).To(BeFalse(), "the current version must not complete a different-version request")
			cv.Status.Desired = configv1.Release{Version: "4.18.2", Architecture: "Multi", Image: "multi-target"}
			cv.Status.History = append([]configv1.UpdateHistory{{Version: "4.18.2", Image: "multi-target", State: configv1.PartialUpdate}}, cv.Status.History...)
			Expect(c.HasUpgradeCompleted(cv, uc)).To(BeFalse(), "an incomplete target payload must not be reported complete")
			cv.Status.History[0].State = configv1.CompletedUpdate
			Expect(c.HasUpgradeCompleted(cv, uc)).To(BeTrue(), "recognize the completed target payload")
		})
	})

	DescribeTable("Waiting for an existing rollout", func(history []configv1.UpdateHistory) {
		cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1"}, History: history}}
		done, err := (&clusterVersionClient{}).runUpgradeWithArchitecture(context.Background(), cv, uc)
		Expect(err).NotTo(HaveOccurred(), "wait for the existing rollout without an error")
		Expect(done).To(BeFalse(), "migration must wait for the existing rollout to complete")
	},
		Entry("when no history is available", []configv1.UpdateHistory(nil)),
		Entry("when the latest update is partial", []configv1.UpdateHistory{{Version: "4.18.1", State: configv1.PartialUpdate}}),
	)
})

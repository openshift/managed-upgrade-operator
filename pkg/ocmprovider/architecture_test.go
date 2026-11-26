package ocmprovider

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
)

var _ = Describe("OCM architecture policies", func() {
	var cluster *cmv1.Cluster
	BeforeEach(func() {
		var err error
		cluster, err = cmv1.NewCluster().Version(cmv1.NewVersion().ID("4.18.1").ChannelGroup("fast")).Build()
		Expect(err).NotTo(HaveOccurred())
	})

	It("maps the API's Multi target without inferring a channel or image", func() {
		policy, err := cmv1.UnmarshalUpgradePolicy([]byte(`{"id":"migration","version":"4.18.1","architecture":"Multi","schedule_type":"manual","next_run":"2026-11-24T12:00:00Z"}`))
		Expect(err).NotTo(HaveOccurred())
		specs, err := buildUpgradeConfigSpecs(policy, cluster, upgradev1alpha1.OSD)
		Expect(err).NotTo(HaveOccurred())
		Expect(specs).To(HaveLen(1))
		Expect(specs[0].Desired).To(Equal(upgradev1alpha1.Update{Version: "4.18.1", Architecture: configv1.ClusterVersionArchitectureMulti}))
		Expect(specs[0].UpgradeAt).To(Equal("2026-11-24T12:00:00Z"))
		Expect(specs[0].CapacityReservation).To(BeTrue())
		state, err := cmv1.NewUpgradePolicyState().Value(cmv1.UpgradePolicyStateValue("scheduled")).Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(isActionableUpgradePolicy(policy, state)).To(BeTrue())
	})

	It("preserves channel inference for ordinary version upgrades", func() {
		policy, err := cmv1.UnmarshalUpgradePolicy([]byte(`{"version":"4.19.1"}`))
		Expect(err).NotTo(HaveOccurred())
		specs, err := buildUpgradeConfigSpecs(policy, cluster, upgradev1alpha1.OSD)
		Expect(err).NotTo(HaveOccurred())
		Expect(specs[0].Desired).To(Equal(upgradev1alpha1.Update{Version: "4.19.1", Channel: "fast-4.19"}))
	})

	It("rejects unsupported architecture values rather than treating them as version upgrades", func() {
		for _, architecture := range []string{"multi", "amd64", "future"} {
			policy, err := cmv1.UnmarshalUpgradePolicy([]byte(fmt.Sprintf(`{"version":"4.18.1","architecture":%q}`, architecture)))
			Expect(err).NotTo(HaveOccurred())
			specs, err := buildUpgradeConfigSpecs(policy, cluster, upgradev1alpha1.OSD)
			Expect(err).To(MatchError(ContainSubstring("unsupported architecture")))
			Expect(specs).To(BeNil())
		}
	})
})

package ocmprovider

import (
	"fmt"
	"os"

	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	mockOcm "github.com/openshift/managed-upgrade-operator/pkg/ocm/mocks"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const (
	TEST_CLUSTER_ID                 = "111111-2222222-3333333-4444444"
	TEST_OPERATOR_NAMESPACE         = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
)

var _ = Describe("OCM Provider", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		mockOcmClient  *mockOcm.MockOcmClient
		provider       *ocmProvider
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		mockOcmClient = mockOcm.NewMockOcmClient(mockCtrl)
		provider = &ocmProvider{
			client:      mockKubeClient,
			ocmClient:   mockOcmClient,
			upgradeType: "TEST_UPGRADE_TYPE",
		}
		_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Inferring the upgrade channel", func() {
		It("Sets the channel based on the channel group and version", func() {
			version := "4.9.1"
			channelGroup := "fast"
			channel, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(*channel).To(Equal("fast-4.9"))
			Expect(err).To(BeNil())
		})
		It("Errors if the version is not parseable", func() {
			version := "crashme"
			channelGroup := "fast"
			_, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(err).NotTo(BeNil())
		})
		It("Sets the channel to stable if the channel group is empty", func() {
			version := "4.9.1"
			channelGroup := ""
			channel, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(*channel).To(Equal("stable-4.9"))
			Expect(err).To(BeNil())
		})
		It("Rounds up to next even number for EUS channel groups with odd minor version", func() {
			version := "4.17.1"
			channelGroup := "eus"
			channel, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(*channel).To(Equal("eus-4.18"))
			Expect(err).To(BeNil())
		})
		It("Keeps even minor version for EUS channel groups", func() {
			version := "4.18.1"
			channelGroup := "eus"
			channel, err := inferUpgradeChannelFromChannelGroup(channelGroup, version)
			Expect(*channel).To(Equal("eus-4.18"))
			Expect(err).To(BeNil())
		})
	})

	// TODO: Rewrite these tests using httptest.Server approach
	// The SDK migration changed return types to *cmv1.UpgradePoliciesListResponse
	// which cannot be manually constructed in mocks - they must come from HTTP responses.
	// See pkg/ocm/client_test.go for examples of the httptest.Server pattern.
	Context("SDK-based tests requiring conversion", func() {
		It("need to be rewritten with httptest.Server", func() {
			Skip("These tests require conversion to httptest.Server approach after SDK migration. See pkg/ocm/client_test.go for pattern.")
		})
	})

	Context("Checking if an upgrade policy is actionable with provider errors", func() {
		It("Errors if the provider is unavailable", func() {
			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(nil, fmt.Errorf("fake error")),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrProviderUnavailable))
			Expect(specs).To(BeNil())
		})

		It("Errors if the internal cluster ID can't be retrieved", func() {
			gomock.InOrder(
				mockOcmClient.EXPECT().GetCluster().Return(nil, ocm.ErrClusterIdNotFound),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ocm.ErrClusterIdNotFound))
			Expect(specs).To(BeNil())
		})
	})
})

package ocm

import (
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/managed-upgrade-operator/util/mocks"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_CLUSTER_ID       = "111111-2222222-3333333-4444444"
	TEST_POLICY_ID_MANUAL = "aaaaaa-bbbbbb-cccccc-dddddd"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE         = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE  = "OSD"
	TEST_UPGRADEPOLICY_TIME         = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60

	// OCM test constants
	TEST_OCM_SERVER_URL = "https://fakeapi.openshift.com"
)

var _ = Describe("OCM Client", func() {

	var (
		mockCtrl                   *gomock.Controller
		mockKubeClient             *mocks.MockClient
		httpClient                 *resty.Client
		clusterListResponse        ClusterList
		upgradePolicyListResponse  UpgradePolicyList
		upgradePolicyStateResponse UpgradePolicyState
		oc                         ocmClient
	)

	BeforeSuite(func() {
		httpClient = resty.New()
		httpmock.ActivateNonDefault(httpClient.GetClient())
	})

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		ocmServerUrl, _ := url.Parse(TEST_OCM_SERVER_URL)

		oc = ocmClient{
			client:     mockKubeClient,
			ocmBaseUrl: ocmServerUrl,
			httpClient: httpClient,
		}

		clusterListResponse = ClusterList{
			Kind:  "ClusterList",
			Page:  1,
			Size:  1,
			Total: 1,
			Items: []ClusterInfo{
				{
					Id: TEST_CLUSTER_ID,
					Version: ClusterVersion{
						Id:           "4.4.4",
						ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
					},
					NodeDrainGracePeriod: NodeDrainGracePeriod{
						Value: TEST_UPGRADEPOLICY_PDB_TIME,
						Unit:  "minutes",
					},
				},
			},
		}
		upgradePolicyListResponse = UpgradePolicyList{
			Kind:  "UpgradePolicyList",
			Page:  1,
			Size:  1,
			Total: 1,
			Items: []UpgradePolicy{
				{
					Id:           TEST_POLICY_ID_MANUAL,
					Kind:         "UpgradePolicy",
					Href:         "test",
					Schedule:     "test",
					ScheduleType: "manual",
					UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
					Version:      TEST_UPGRADEPOLICY_VERSION,
					NextRun:      TEST_UPGRADEPOLICY_TIME,
					ClusterId:    TEST_CLUSTER_ID,
				},
			},
		}
		upgradePolicyStateResponse = UpgradePolicyState{
			Kind:        "UpgradePolicyState",
			Href:        "test",
			Value:       "pending",
			Description: "Upgrade is pending",
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		httpmock.Reset()
	})

	Context("When getting cluster info", func() {
		var cv *configv1.ClusterVersion

		BeforeEach(func() {
			cv = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID,
				},
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		})

		It("returns the correct info", func() {

			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)

			result, err := oc.GetCluster()
			Expect(*result).To(Equal(clusterListResponse.Items[0]))
			Expect(err).To(BeNil())
		})
	})

	Context("When getting upgrade policies", func() {
		It("returns the correct info", func() {

			upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
			upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

			result, err := oc.GetClusterUpgradePolicies(TEST_CLUSTER_ID)

			Expect(*result).To(Equal(upgradePolicyListResponse))
			Expect(err).To(BeNil())
		})
	})

	Context("When getting upgrade policy state", func() {
		It("returns the correct info", func() {

			upsResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyStateResponse)
			upsUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH, TEST_POLICY_ID_MANUAL, STATE_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, upsUrl, upsResponder)

			result, err := oc.GetClusterUpgradePolicyState(TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID)

			Expect(*result).To(Equal(upgradePolicyStateResponse))
			Expect(err).To(BeNil())
		})
	})

})

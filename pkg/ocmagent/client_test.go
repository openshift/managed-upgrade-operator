package ocmagent

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"

	"github.com/openshift/managed-upgrade-operator/pkg/ocm"

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
	TEST_UPGRADEPOLICY_ADDON        = "ADDON"
	TEST_UPGRADEPOLICY_TIME         = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60

	// OCM test constants
	TEST_OCM_SERVER_URL = "https://ocm-agent.svc.cluster.info"
)

var _ = Describe("OCM Client", func() {

	var (
		mockCtrl                   *gomock.Controller
		httpClient                 *resty.Client
		clusterInfoResponse        ocm.ClusterInfo
		upgradePolicyListResponse  []ocm.UpgradePolicy
		upgradePolicyStateResponse ocm.UpgradePolicyState
		oc                         ocmClient
	)

	BeforeSuite(func() {
		httpClient = resty.New()
		httpmock.ActivateNonDefault(httpClient.GetClient())
	})

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		ocmServerUrl, _ := url.Parse(TEST_OCM_SERVER_URL)

		oc = ocmClient{
			ocmBaseUrl: ocmServerUrl,
			httpClient: httpClient,
		}

		clusterInfoResponse = ocm.ClusterInfo{
			Id: TEST_CLUSTER_ID,
			Version: ocm.ClusterVersion{
				Id:           "4.4.4",
				ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
			},
			NodeDrainGracePeriod: ocm.NodeDrainGracePeriod{
				Value: TEST_UPGRADEPOLICY_PDB_TIME,
				Unit:  "minutes",
			},
		}

		upgradePolicyListResponse = []ocm.UpgradePolicy{
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
			{
				Id:           TEST_POLICY_ID_MANUAL,
				Kind:         "UpgradePolicy",
				Href:         "test",
				Schedule:     "test",
				ScheduleType: "manual",
				UpgradeType:  TEST_UPGRADEPOLICY_ADDON,
				Version:      TEST_UPGRADEPOLICY_VERSION,
				NextRun:      TEST_UPGRADEPOLICY_TIME,
				ClusterId:    TEST_CLUSTER_ID,
			},
		}

		upgradePolicyStateResponse = ocm.UpgradePolicyState{
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
		It("returns the correct info", func() {
			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterInfoResponse)
			httpmock.RegisterResponder(http.MethodGet, TEST_OCM_SERVER_URL, clResponder)

			result, err := oc.GetCluster()

			Expect(*result).To(Equal(clusterInfoResponse))
			Expect(err).To(BeNil())
		})
	})

	Context("When getting upgrade policies", func() {
		It("retrieves OSD type only", func() {
			upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
			upUrl := strings.Join([]string{TEST_OCM_SERVER_URL, UPGRADEPOLICIES_PATH}, "/")
			httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

			result, err := oc.GetClusterUpgradePolicies(TEST_CLUSTER_ID)

			Expect(result).To(Equal(&ocm.UpgradePolicyList{
				Kind: "UpgradePolicyList",
				Page: 1,
				Size: int64(len(upgradePolicyListResponse)),
				Total: int64(len(upgradePolicyListResponse)),
				Items: upgradePolicyListResponse,
			}))
			Expect(err).To(BeNil())
		})
	})

	Context("When getting upgrade policy state", func() {
		It("returns the correct info", func() {

			upsResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyStateResponse)
			upsUrl := strings.Join([]string{TEST_OCM_SERVER_URL, UPGRADEPOLICIES_PATH, TEST_POLICY_ID_MANUAL, STATE_V1_PATH}, "/")
			httpmock.RegisterResponder(http.MethodGet, upsUrl, upsResponder)

			result, err := oc.GetClusterUpgradePolicyState(TEST_POLICY_ID_MANUAL, TEST_CLUSTER_ID)

			Expect(*result).To(Equal(upgradePolicyStateResponse))
			Expect(err).To(BeNil())
		})
	})

})

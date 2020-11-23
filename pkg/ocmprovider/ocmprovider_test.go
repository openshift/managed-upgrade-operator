package ocmprovider

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/jarcoal/httpmock"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

const (
	TEST_CLUSTER_ID                  = "111111-2222222-3333333-4444444"
	TEST_CLUSTER_ID_MULTI_POLICIES   = "444444-333333-222222-111111"
	TEST_POLICY_ID_MANUAL            = "aaaaaa-bbbbbb-cccccc-dddddd"
	TEST_POLICY_ID_AUTOMATIC         = "aaaaaa-bbbbbb-cccccc-dddddd"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE                = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE         = "OSD"
	TEST_UPGRADEPOLICY_TIME                = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING = "2020-05-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION             = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP        = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME            = 60

	// OCM test constants
	TEST_OCM_SERVER_URL = "https://fakeapi.openshift.com"
)

var _ = Describe("OCM Provider", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		provider       *ocmProvider
		httpClient				 *resty.Client
	)

	BeforeSuite(func() {
		httpClient = resty.New()
		httpmock.ActivateNonDefault(httpClient.GetClient())
	})

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		ocmServerUrl, _ := url.Parse(TEST_OCM_SERVER_URL)

		provider = &ocmProvider{
			client:     mockKubeClient,
			ocmBaseUrl: ocmServerUrl,
			httpClient: httpClient,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		httpmock.Reset()
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

	})

	Context("Getting upgrade policies", func() {
		var (
			cv                        *configv1.ClusterVersion
			clusterListResponse       clusterList
			upgradePolicyListResponse upgradePolicyList
		)

		BeforeEach(func() {
			cv = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID,
				},
			}
			clusterListResponse = clusterList{
				Kind:  "ClusterList",
				Page:  1,
				Size:  1,
				Total: 1,
				Items: []clusterInfo{
					{
						Id: TEST_CLUSTER_ID,
						Version: clusterVersion{
							Id:           "4.4.4",
							ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
						},
						NodeDrainGracePeriod: nodeDrainGracePeriod{
							Value: TEST_UPGRADEPOLICY_PDB_TIME,
							Unit:  "minutes",
						},
					},
				},
			}
			upgradePolicyListResponse = upgradePolicyList{
				Kind:  "UpgradePolicyList",
				Page:  1,
				Size:  1,
				Total: 1,
				Items: []upgradePolicy{
					{
						Id:           TEST_POLICY_ID_MANUAL,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "test",
						ScheduleType: "manual",
						UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME,
						ClusterId: TEST_CLUSTER_ID,
					},
				},
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		})

		It("Returns specs if they exist", func() {
			expectedSpec := v1alpha1.UpgradeConfigSpec{
				Desired: v1alpha1.Update{
					Version: TEST_UPGRADEPOLICY_VERSION,
					Channel: TEST_UPGRADEPOLICY_CHANNELGROUP + "-4.4",
				},
				UpgradeAt:            TEST_UPGRADEPOLICY_TIME,
				PDBForceDrainTimeout: TEST_UPGRADEPOLICY_PDB_TIME,
				Type:                 TEST_UPGRADEPOLICY_UPGRADETYPE,
			}

			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upgradePolicyListResponse)
			upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(expectedSpec))
		})

		It("Returns next occurring spec if multiple exist", func() {
			expectedSpec := v1alpha1.UpgradeConfigSpec{
				Desired: v1alpha1.Update{
					Version: TEST_UPGRADEPOLICY_VERSION,
					Channel: TEST_UPGRADEPOLICY_CHANNELGROUP + "-4.4",
				},
				UpgradeAt:            TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING,
				PDBForceDrainTimeout: TEST_UPGRADEPOLICY_PDB_TIME,
				Type:                 TEST_UPGRADEPOLICY_UPGRADETYPE,
			}

			multiPolicyResponse := upgradePolicyList{
				Kind:  "UpgradePolicyList",
				Page:  1,
				Size:  2,
				Total: 2,
				Items: []upgradePolicy{
					{
						Id:           TEST_POLICY_ID_MANUAL,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "test",
						ScheduleType: "manual",
						UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME,
						ClusterId: TEST_CLUSTER_ID_MULTI_POLICIES,
					},
					{
						Id:           TEST_POLICY_ID_AUTOMATIC,
						Kind:         "UpgradePolicy",
						Href:         "test",
						Schedule:     "3 5 5 * *",
						ScheduleType: "automatic",
						UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
						Version:      TEST_UPGRADEPOLICY_VERSION,
						NextRun:      TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING,
						ClusterId: TEST_CLUSTER_ID_MULTI_POLICIES,
					},
				},
			}

			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, multiPolicyResponse)
			upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(expectedSpec))
		})

		It("Returns no specs if there are no policies", func() {
			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			upResponse := upgradePolicyList{
				Kind:  "UpgradePolicyList",
				Page:  1,
				Size:  0,
				Total: 0,
				Items: []upgradePolicy{},
			}
			upResponder, _ := httpmock.NewJsonResponder(http.StatusOK, upResponse)
			upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(BeEmpty())
		})

		It("Errors if the provider is unavailable", func() {

			clResponder := httpmock.NewErrorResponder(fmt.Errorf("fake error"))
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrProviderUnavailable))
			Expect(specs).To(BeNil())
		})

		It("Errors if the internal cluster ID can't be retrieved", func() {

			noClustersResponse := clusterList{
				Kind:  "ClusterList",
				Page:  1,
				Size:  0,
				Total: 0,
				Items: []clusterInfo{},
			}
			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, noClustersResponse)
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrClusterIdNotFound))
			Expect(specs).To(BeNil())
		})


		It("Errors if the policy response can't be parsed", func() {

			clResponder, _ := httpmock.NewJsonResponder(http.StatusOK, clusterListResponse)
			clUrl := path.Join(CLUSTERS_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, clUrl, clResponder)

			upResponder := httpmock.NewStringResponder(http.StatusOK, "this isnt a policy response")
			upUrl := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
			httpmock.RegisterResponder(http.MethodGet, upUrl, upResponder)

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *cv).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrRetrievingPolicies))
			Expect(specs).To(BeNil())
		})

	})
})

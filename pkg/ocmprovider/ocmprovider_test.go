package ocmprovider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"k8s.io/apimachinery/pkg/types"

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
	TEST_CLUSTER_ID_WITH_NO_POLICIES = "555555-666666-777777-888888"
	TEST_CLUSTER_ID_FOR_BAD_REPLY    = "999999-aaaaaa-bbbbbb-cccccc"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE                = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE         = "OSD"
	TEST_UPGRADEPOLICY_TIME                = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING = "2020-05-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION             = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP        = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME            = 60
)

var _ = Describe("OCM Provider", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		provider       *ocmProvider
		ocmServer      *httptest.Server
	)

	BeforeEach(func() {
		ocmServer = ocmServerMock()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)

		ocmServerUrl, _ := url.Parse(ocmServer.URL)

		provider = &ocmProvider{
			client:     mockKubeClient,
			ocmBaseUrl: ocmServerUrl,
			httpClient: &http.Client{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		ocmServer.Close()
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
		var clusterVersion *configv1.ClusterVersion
		BeforeEach(func() {
			clusterVersion = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID,
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

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(expectedSpec))
		})

		It("Returns next occurring spec if multiple exist", func() {
			multiPolicyClusterVersion := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID_MULTI_POLICIES,
				},
			}
			expectedSpec := v1alpha1.UpgradeConfigSpec{
				Desired: v1alpha1.Update{
					Version: TEST_UPGRADEPOLICY_VERSION,
					Channel: TEST_UPGRADEPOLICY_CHANNELGROUP + "-4.4",
				},
				UpgradeAt:            TEST_UPGRADEPOLICY_TIME_NEXT_OCCURRING,
				PDBForceDrainTimeout: TEST_UPGRADEPOLICY_PDB_TIME,
				Type:                 TEST_UPGRADEPOLICY_UPGRADETYPE,
			}

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *multiPolicyClusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(ContainElement(expectedSpec))
		})

		It("Returns no specs if there are no policies", func() {
			noPolicyClusterVersion := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID_WITH_NO_POLICIES,
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *noPolicyClusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(BeNil())
			Expect(specs).To(BeEmpty())
		})

		It("Errors if the provider is unavailable", func() {
			u, _ := url.Parse("http://doesnotexist.example.com")
			provider.ocmBaseUrl = u
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrProviderUnavailable))
			Expect(specs).To(BeNil())
		})

		It("Errors if the internal cluster ID can't be retrieved", func() {
			clusterVersion = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: "not a cluster ID we know",
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrClusterIdNotFound))
			Expect(specs).To(BeNil())
		})

		It("Errors if the internal cluster ID can't be retrieved", func() {
			clusterVersion = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: "not a cluster ID we know",
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrClusterIdNotFound))
			Expect(specs).To(BeNil())
		})

		It("Errors if the policy response can't be parsed", func() {
			noPolicyClusterVersion := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID_FOR_BAD_REPLY,
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *noPolicyClusterVersion).Return(nil),
			)
			specs, err := provider.Get()
			Expect(err).To(Equal(ErrRetrievingPolicies))
			Expect(specs).To(BeNil())
		})

	})
})

func ocmServerMock() *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc(CLUSTERS_V1_PATH, clustersMock)

	upPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(upPolicyPath, policyMock)

	multiUpPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_MULTI_POLICIES, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(multiUpPolicyPath, multiPolicyMock)

	noPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_WITH_NO_POLICIES, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(noPolicyPath, noPolicyMock)

	garbagePath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_FOR_BAD_REPLY, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(garbagePath, garbageMock)

	srv := httptest.NewServer(handler)
	return srv
}

func clustersMock(w http.ResponseWriter, r *http.Request) {
	response := clusterList{
		Kind:  "ClusterList",
		Page:  1,
		Size:  1,
		Total: 1,
		Items: []clusterInfo{
			{
				Id: "",
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

	// Return different responses based on the cluster ID searched for
	clusterSearch := r.URL.Query().Get("search")

	// Return a cluster ID that'll have one upgrade policy
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID) {
		response.Items[0].Id = TEST_CLUSTER_ID
	}
	// Return a cluster ID that'll have multiple upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID_MULTI_POLICIES) {
		response.Items[0].Id = TEST_CLUSTER_ID_MULTI_POLICIES
	}
	// Return a cluster ID that'll have no upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID_WITH_NO_POLICIES) {
		response.Items[0].Id = TEST_CLUSTER_ID_WITH_NO_POLICIES
	}
	// Return a cluster ID that'll have no upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID_FOR_BAD_REPLY) {
		response.Items[0].Id = TEST_CLUSTER_ID_FOR_BAD_REPLY
	}

	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func policyMock(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyList{
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
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func multiPolicyMock(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyList{
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
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func noPolicyMock(w http.ResponseWriter, r *http.Request) {
	response := upgradePolicyList{
		Kind:  "UpgradePolicyList",
		Page:  1,
		Size:  0,
		Total: 0,
		Items: []upgradePolicy{},
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

func garbageMock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, "this is not json at all {")
}

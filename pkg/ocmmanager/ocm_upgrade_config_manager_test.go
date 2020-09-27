package ocmmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_CLUSTER_ID = "111111-2222222-3333333-4444444"
	TEST_POLICY_ID  = "aaaaaa-bbbbbb-cccccc-dddddd"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE         = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE  = "OSD"
	TEST_UPGRADEPOLICY_TIME         = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
)

var _ = Describe("OCMUpgradeConfigManager", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		manager        *osdUpgradeConfigManager
		ocmServer      *httptest.Server
	)

	BeforeEach(func() {
		ocmServer = ocmServerMock()
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)

		cfg := &ocmUpgradeConfigManagerConfig{
			ConfigManagerConfig: configManagerConfig{
				OcmBaseURL:           ocmServer.URL,
				WatchIntervalMinutes: 1,
			},
		}
		manager = &osdUpgradeConfigManager{
			client:     mockKubeClient,
			config:     cfg,
			httpClient: &http.Client{},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
		ocmServer.Close()
	})

	Context("stuff", func() {
		var clusterVersion *configv1.ClusterVersion
		var upgradeConfigs *upgradev1alpha1.UpgradeConfigList
		BeforeEach(func() {
			clusterVersion = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID,
				},
			}
			upgradeConfigs = &upgradev1alpha1.UpgradeConfigList{
				Items: []upgradev1alpha1.UpgradeConfig{},
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		})

		It("stuff", func() {

			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *upgradeConfigs).Return(nil),
				mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(UPGRADECONFIG_CR_NAME))
						Expect(uc.Namespace).To(Equal(TEST_OPERATOR_NAMESPACE))
						Expect(string(uc.Spec.Type)).To(Equal(TEST_UPGRADEPOLICY_UPGRADETYPE))
						Expect(uc.Spec.Desired.Version).To(Equal(TEST_UPGRADEPOLICY_VERSION))
						Expect(uc.Spec.Desired.Channel).To(HavePrefix(TEST_UPGRADEPOLICY_CHANNELGROUP))
						return nil
					}),
			)
			_, err := manager.RefreshUpgradeConfig()
			Expect(err).To(BeNil())

		})
	})
})

func ocmServerMock() *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc(CLUSTERS_V1_PATH, clustersMock)

	upPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(upPolicyPath, policyMock)

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
				Id: TEST_CLUSTER_ID,
				Version: clusterVersion{
					Id:           "4.4.4",
					ChannelGroup: TEST_UPGRADEPOLICY_CHANNELGROUP,
				},
			},
		},
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
				Id:                   TEST_POLICY_ID,
				Kind:                 "UpgradePolicy",
				Href:                 "test",
				Schedule:             "test",
				ScheduleType:         "manual",
				UpgradeType:          TEST_UPGRADEPOLICY_UPGRADETYPE,
				Version:              TEST_UPGRADEPOLICY_VERSION,
				NextRun:              TEST_UPGRADEPOLICY_TIME,
				NodeDrainGracePeriod: nodeDrainGracePeriod{},
				ClusterId:            TEST_CLUSTER_ID,
			},
		},
	}
	responseJson, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, string(responseJson))
}

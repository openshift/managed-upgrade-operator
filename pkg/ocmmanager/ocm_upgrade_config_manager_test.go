package ocmmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	k8serrs "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	TEST_CLUSTER_ID                  = "111111-2222222-3333333-4444444"
	TEST_POLICY_ID                   = "aaaaaa-bbbbbb-cccccc-dddddd"
	TEST_CLUSTER_ID_WITH_NO_POLICIES = "555555-666666-777777-888888"

	// Upgrade policy constants
	TEST_OPERATOR_NAMESPACE         = "test-managed-upgrade-operator"
	TEST_UPGRADEPOLICY_UPGRADETYPE  = "OSD"
	TEST_UPGRADEPOLICY_TIME         = "2020-06-20T00:00:00Z"
	TEST_UPGRADEPOLICY_VERSION      = "4.4.5"
	TEST_UPGRADEPOLICY_CHANNELGROUP = "fast"
	TEST_UPGRADEPOLICY_PDB_TIME     = 60
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

	Context("Refreshing the UpgradeConfig", func() {
		var clusterVersion *configv1.ClusterVersion
		BeforeEach(func() {
			clusterVersion = &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID,
				},
			}
			_ = os.Setenv("OPERATOR_NAMESPACE", TEST_OPERATOR_NAMESPACE)
		})

		It("Creates a new policy if one doesn't exist", func() {
			upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{
				Items: []upgradev1alpha1.UpgradeConfig{},
			}
			notFound := k8serrs.NewNotFound(schema.GroupResource{}, UPGRADECONFIG_CR_NAME)
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *upgradeConfigs).Return(nil),
				mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Return(notFound),
				mockKubeClient.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(UPGRADECONFIG_CR_NAME))
						Expect(uc.Namespace).To(Equal(TEST_OPERATOR_NAMESPACE))
						Expect(string(uc.Spec.Type)).To(Equal(TEST_UPGRADEPOLICY_UPGRADETYPE))
						Expect(uc.Spec.Desired.Version).To(Equal(TEST_UPGRADEPOLICY_VERSION))
						Expect(uc.Spec.Desired.Channel).To(HavePrefix(TEST_UPGRADEPOLICY_CHANNELGROUP))
						Expect(uc.Spec.PDBForceDrainTimeout).To(Equal(int32(TEST_UPGRADEPOLICY_PDB_TIME)))
						return nil
					}),
			)
			changed, err := manager.RefreshUpgradeConfig()
			Expect(err).To(BeNil())
			Expect(changed).To(BeTrue())
		})

		It("Updates an existing policy if one already exists", func() {
			testUCName := "a unique name"
			testUCNS := "a unique namespace"
			upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{
				Items: []upgradev1alpha1.UpgradeConfig{
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      testUCName,
							Namespace: testUCNS,
						},
						Spec: upgradev1alpha1.UpgradeConfigSpec{
							Desired: upgradev1alpha1.Update{
								Version: "a unique ver",
								Channel: "a unique chan",
							},
							UpgradeAt:            "some time",
							PDBForceDrainTimeout: 10,
							Type:                 "osd",
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *upgradeConfigs).Return(nil),
				mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(testUCName))
						Expect(uc.Namespace).To(Equal(testUCNS))
						Expect(string(uc.Spec.Type)).To(Equal(TEST_UPGRADEPOLICY_UPGRADETYPE))
						Expect(uc.Spec.Desired.Version).To(Equal(TEST_UPGRADEPOLICY_VERSION))
						Expect(uc.Spec.Desired.Channel).To(HavePrefix(TEST_UPGRADEPOLICY_CHANNELGROUP))
						Expect(uc.Spec.PDBForceDrainTimeout).To(Equal(int32(TEST_UPGRADEPOLICY_PDB_TIME)))
						return nil
					}),
			)
			changed, err := manager.RefreshUpgradeConfig()
			Expect(err).To(BeNil())
			Expect(changed).To(BeTrue())
		})

		It("Does not update the UpgradeConfig if the policy hasn't changed", func() {
			testUCName := "a unique name"
			testUCNS := "a unique namespace"
			upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{
				Items: []upgradev1alpha1.UpgradeConfig{
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      testUCName,
							Namespace: testUCNS,
						},
						Spec: upgradev1alpha1.UpgradeConfigSpec{
							Desired: upgradev1alpha1.Update{
								Version: TEST_UPGRADEPOLICY_VERSION,
								Channel: TEST_UPGRADEPOLICY_CHANNELGROUP + "-4.4",
							},
							UpgradeAt:            TEST_UPGRADEPOLICY_TIME,
							PDBForceDrainTimeout: TEST_UPGRADEPOLICY_PDB_TIME,
							Type:                 TEST_UPGRADEPOLICY_UPGRADETYPE,
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *clusterVersion).Return(nil),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *upgradeConfigs).Return(nil),
			)
			changed, err := manager.RefreshUpgradeConfig()
			Expect(err).To(BeNil())
			Expect(changed).To(BeFalse())
		})

		It("Deletes existing UpgradeConfigs if there is no policy returned", func() {
			// Override cluster ID with one that'll return no policies from our mock server
			noPolicyClusterVersion := &configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					ClusterID: TEST_CLUSTER_ID_WITH_NO_POLICIES,
				},
			}
			upgradeConfigs := &upgradev1alpha1.UpgradeConfigList{
				Items: []upgradev1alpha1.UpgradeConfig{
					{
						ObjectMeta: v1.ObjectMeta{
							Name:      UPGRADECONFIG_CR_NAME,
							Namespace: TEST_OPERATOR_NAMESPACE,
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()).SetArg(2, *noPolicyClusterVersion).Return(nil),
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *upgradeConfigs).Return(nil),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, uc *upgradev1alpha1.UpgradeConfig) error {
						Expect(uc.Name).To(Equal(UPGRADECONFIG_CR_NAME))
						Expect(uc.Namespace).To(Equal(TEST_OPERATOR_NAMESPACE))
						return nil
					}),
			)
			changed, err := manager.RefreshUpgradeConfig()
			Expect(err).To(BeNil())
			Expect(changed).To(BeTrue())
		})
	})
})

func ocmServerMock() *httptest.Server {
	handler := http.NewServeMux()
	handler.HandleFunc(CLUSTERS_V1_PATH, clustersMock)

	upPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(upPolicyPath, policyMock)

	noPolicyPath := path.Join(CLUSTERS_V1_PATH, TEST_CLUSTER_ID_WITH_NO_POLICIES, UPGRADEPOLICIES_V1_PATH)
	handler.HandleFunc(noPolicyPath, noPolicyMock)

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
			},
		},
	}

	// Return different responses based on the cluster ID searched for
	clusterSearch := r.URL.Query().Get("search")

	// Return a cluster ID that'll have some upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID) {
		response.Items[0].Id = TEST_CLUSTER_ID
	}
	// Return a cluster ID that'll have no upgrade policies
	if strings.Contains(clusterSearch, TEST_CLUSTER_ID_WITH_NO_POLICIES) {
		response.Items[0].Id = TEST_CLUSTER_ID_WITH_NO_POLICIES
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
				Id:           TEST_POLICY_ID,
				Kind:         "UpgradePolicy",
				Href:         "test",
				Schedule:     "test",
				ScheduleType: "manual",
				UpgradeType:  TEST_UPGRADEPOLICY_UPGRADETYPE,
				Version:      TEST_UPGRADEPOLICY_VERSION,
				NextRun:      TEST_UPGRADEPOLICY_TIME,
				NodeDrainGracePeriod: nodeDrainGracePeriod{
					Value: TEST_UPGRADEPOLICY_PDB_TIME,
					Unit:  "minutes",
				},
				ClusterId: TEST_CLUSTER_ID,
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

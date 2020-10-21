package clusterversion

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	testStructs "github.com/openshift/managed-upgrade-operator/util/mocks/structs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ClusterVersion client and utils", func() {

	var (
		cvClient          ClusterVersion
		mockCtrl          *gomock.Controller
		mockKubeClient    *mocks.MockClient
		testName          string
		upgradeConfig     *upgradev1alpha1.UpgradeConfig
		upgradeConfigName types.NamespacedName
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		cvClient = &clusterVersionClient{mockKubeClient}
		testName = "testClusterversion"
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
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{{ObjectMeta: metav1.ObjectMeta{Name: testName}}},
				}).Return(nil),
			)
			clusterVersion, err := cvClient.GetClusterVersion()
			Expect(clusterVersion).To(Not(BeNil()))
			Expect(clusterVersion.Name).To(Equal(testName))
			Expect(err).Should(BeNil())
		})
		It("should error if ClusterVersion resource is not found", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{},
				}).Return(nil),
			)
			clusterVersion, err := cvClient.GetClusterVersion()
			Expect(clusterVersion).To(BeNil())
			Expect(err).Should(Not(BeNil()))
		})
	})

	Context("When the cluster's desired version matches the UpgradeConfig's", func() {
		It("Indicates the upgrade has commenced", func() {
			clusterVersion := configv1.ClusterVersion{
				Spec: configv1.ClusterVersionSpec{
					Channel:       upgradeConfig.Spec.Desired.Channel,
					DesiredUpdate: &configv1.Update{Version: upgradeConfig.Spec.Desired.Version},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, configv1.ClusterVersionList{
					Items: []configv1.ClusterVersion{clusterVersion},
				}).Return(nil),
			)
			hasCommenced, err := cvClient.HasUpgradeCommenced(upgradeConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasCommenced).To(BeTrue())
		})
	})
})

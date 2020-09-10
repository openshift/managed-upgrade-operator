package clusterversion

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ClusterVersion client and utils", func() {

	var (
		cvClient       ClusterVersion
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		testName       string
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		cvClient = &clusterVersionClient{mockKubeClient}
		testName = "testClusterversion"
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
})

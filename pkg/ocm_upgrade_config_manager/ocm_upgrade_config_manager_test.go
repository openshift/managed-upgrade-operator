package ocm_upgrade_config_manager

import (
"github.com/golang/mock/gomock"
. "github.com/onsi/ginkgo"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

var _ = Describe("OCMUpgradeConfigManager", func() {
	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		manager *osdUpgradeConfigManager
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		manager = &osdUpgradeConfigManager{
			client:               mockKubeClient,
			configManagerBuilder: nil,
			httpClient:           nil,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("", func() {
		It("", func() {
		})
	})
})

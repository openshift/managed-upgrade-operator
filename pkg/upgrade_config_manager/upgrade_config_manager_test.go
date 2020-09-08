package upgrade_config_manager

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
)

var _ = Describe("UpgradeConfig Manager", func() {
	var (
		mockCtrl       *gomock.Controller
		//mockKubeClient *mocks.MockClient
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		//mockKubeClient = mocks.NewMockClient(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("", func() {
		It("", func() {
		})
	})
})

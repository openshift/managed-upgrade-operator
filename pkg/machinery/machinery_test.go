package machinery

import (
	"fmt"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"

	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

var _ = Describe("Machinery client and utils", func() {

	var (
		mockCtrl        *gomock.Controller
		mockKubeClient  *mocks.MockClient
		machineryClient Machinery
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		machineryClient = &machinery{}
	})

	Context("When assessing whether all machines are upgraded", func() {
		var configPool *machineconfigapi.MachineConfigPool
		var nodeType = "worker"

		Context("When checking IsUpgrading errors", func() {
			It("reports the error", func() {
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).Return(fmt.Errorf("Fake error"))
				result, err := machineryClient.IsUpgrading(mockKubeClient, nodeType)
				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		Context("When all total machine and updated machine counts match", func() {
			JustBeforeEach(func() {
				configPool = &machineconfigapi.MachineConfigPool{
					Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 5, UpdatedMachineCount: 5},
				}
			})
			It("Reports that all machines are upgraded", func() {
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).SetArg(2, *configPool).Return(nil)
				result, err := machineryClient.IsUpgrading(mockKubeClient, nodeType)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsUpgrading).To(BeFalse())
			})
		})
		Context("When the updated machine count is less than the total machine count", func() {
			JustBeforeEach(func() {
				configPool = &machineconfigapi.MachineConfigPool{
					Status: machineconfigapi.MachineConfigPoolStatus{MachineCount: 3, UpdatedMachineCount: 2},
				}
			})
			It("Reports that all machines are not upgraded", func() {
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: nodeType}, gomock.Any()).SetArg(2, *configPool).Return(nil)
				result, err := machineryClient.IsUpgrading(mockKubeClient, nodeType)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsUpgrading).To(BeTrue())
			})
		})
	})
})

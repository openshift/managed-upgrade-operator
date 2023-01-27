package machinery

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

	AfterEach(func() {
		mockCtrl.Finish()
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

	Context("When assessing if a node is cordoned", func() {
		It("Reports if the node is draining", func() {
			testNode := &corev1.Node{
				Spec: corev1.NodeSpec{
					Unschedulable: true,
					Taints: []corev1.Taint{
						{Effect: corev1.TaintEffectNoSchedule,
							Key: corev1.TaintNodeUnschedulable,
						},
					},
				},
			}
			result := machineryClient.IsNodeCordoned(testNode)
			Expect(result.IsCordoned).To(BeTrue())
		})
		It("Reports if the node is not draining", func() {
			testNode := &corev1.Node{
				Spec: corev1.NodeSpec{},
			}
			result := machineryClient.IsNodeCordoned(testNode)
			Expect(result.IsCordoned).To(BeFalse())
		})
		It("Reports the time the node started draining", func() {
			startTime := &metav1.Time{Time: time.Now()}
			testNode := &corev1.Node{
				Spec: corev1.NodeSpec{
					Unschedulable: true,
					Taints: []corev1.Taint{
						{Effect: corev1.TaintEffectNoSchedule,
							Key:       corev1.TaintNodeUnschedulable,
							TimeAdded: startTime},
					},
				},
			}
			result := machineryClient.IsNodeCordoned(testNode)
			Expect(result.IsCordoned).To(BeTrue())
			Expect(result.AddedAt).To(Equal(startTime))
		})
	})

	Context("When a node has multiple NoSchedule taints", func() {
		startTime := &metav1.Time{Time: time.Now()}
		testNode := &corev1.Node{
			Spec: corev1.NodeSpec{
				Unschedulable: true,
				Taints:        []corev1.Taint{},
			},
		}
		// Ensure that order is independent
		taintTests := [][]corev1.Taint{
			{
				{Effect: corev1.TaintEffectNoSchedule,
					Key:       corev1.TaintNodeUnschedulable,
					TimeAdded: startTime},
				{Effect: corev1.TaintEffectNoSchedule,
					Key: "A different key"},
			},
			{
				{Effect: corev1.TaintEffectNoSchedule,
					Key: "A different key"},
				{Effect: corev1.TaintEffectNoSchedule,
					Key:       corev1.TaintNodeUnschedulable,
					TimeAdded: startTime},
			},
		}
		It("Uses the drain time from the correct taint", func() {
			for _, taints := range taintTests {
				testNode.Spec.Taints = taints
				result := machineryClient.IsNodeCordoned(testNode)
				Expect(result.IsCordoned).To(BeTrue())
				Expect(result.AddedAt).To(Equal(startTime))
			}
		})
	})
})

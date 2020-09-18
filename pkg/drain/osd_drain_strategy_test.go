package drain

import (
	"time"

	"github.com/golang/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OSD Drain Strategy", func() {

	var (
		mockCtrl          *gomock.Controller
		mockKubeClient    *mocks.MockClient
		osdDrain          NodeDrainStrategy
		mockTimedDrainOne *MockTimedDrainStrategy
		mockStrategyOne   *MockDrainStrategy
		mockTimedDrainTwo *MockTimedDrainStrategy
		mockStrategyTwo   *MockDrainStrategy
		nodeDrainConfig   *NodeDrain
	)

	Context("OSD Time Based Drain Strategy execution", func() {
		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			mockKubeClient = mocks.NewMockClient(mockCtrl)
			mockTimedDrainOne = NewMockTimedDrainStrategy(mockCtrl)
			mockStrategyOne = NewMockDrainStrategy(mockCtrl)
			mockTimedDrainTwo = NewMockTimedDrainStrategy(mockCtrl)
			mockStrategyTwo = NewMockDrainStrategy(mockCtrl)
		})
		It("should not error if there are no Strategies", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				&corev1.Node{},
				&NodeDrain{},
				[]TimedDrainStrategy{},
			}
			drainStartedFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-5 * time.Minute)}
			result, err := osdDrain.Execute(drainStartedFiveMinsAgo)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(0))
		})
		It("should execute a Time Based Drain Strategy after the assigned wait duration", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				&corev1.Node{},
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne},
			}
			gomock.InOrder(
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*30),
				mockTimedDrainOne.EXPECT().GetStrategy().Return(mockStrategyOne),
				mockStrategyOne.EXPECT().Execute().Times(1).Return(&DrainStrategyResult{Message: "", HasExecuted: true}, nil),
				mockTimedDrainOne.EXPECT().GetDescription().Times(1).Return("Drain one"),
			)
			drainStartedFortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			result, err := osdDrain.Execute(drainStartedFortyFiveMinsAgo)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(1))
		})
		It("should not execute a Time Based Drain Strategy before the assigned duration", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				&corev1.Node{},
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne},
			}
			gomock.InOrder(
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*60),
				mockTimedDrainOne.EXPECT().GetStrategy().Return(mockStrategyOne),
				mockStrategyOne.EXPECT().Execute().Times(0),
				mockTimedDrainOne.EXPECT().GetDescription().Times(0).Return("Drain one"),
			)
			drainStartedFortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			result, err := osdDrain.Execute(drainStartedFortyFiveMinsAgo)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(0))
		})
		It("should only execute Time Based Drain Strategy at the correct time if multiple strategies exist", func() {
			osdDrain = &osdDrainStrategy{
				mockKubeClient,
				&corev1.Node{},
				&NodeDrain{},
				[]TimedDrainStrategy{mockTimedDrainOne, mockTimedDrainTwo},
			}
			gomock.InOrder(
				mockTimedDrainOne.EXPECT().GetWaitDuration().Return(time.Minute*30),
				mockTimedDrainOne.EXPECT().GetStrategy().Return(mockStrategyOne),
				mockStrategyOne.EXPECT().Execute().Times(1).Return(&DrainStrategyResult{Message: "", HasExecuted: true}, nil),
				mockTimedDrainOne.EXPECT().GetDescription().Times(1).Return("Drain one"),
				mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(time.Minute*60),
				mockTimedDrainTwo.EXPECT().GetStrategy().Return(mockStrategyTwo),
				mockStrategyTwo.EXPECT().Execute().Times(0),
			)
			drainStartedFortyFiveMinsAgo := &metav1.Time{Time: time.Now().Add(-45 * time.Minute)}
			result, err := osdDrain.Execute(drainStartedFortyFiveMinsAgo)
			Expect(result).To(Not(BeNil()))
			Expect(err).To(BeNil())
			Expect(len(result)).To(Equal(1))
		})
	})

	Context("OSD Drain Strategies failures", func() {
		Context("When there are no strategies", func() {
			BeforeEach(func() {
				mockCtrl = gomock.NewController(GinkgoT())
				mockKubeClient = mocks.NewMockClient(mockCtrl)
				nodeDrainConfig = &NodeDrain{
					Timeout: 45,
				}
				osdDrain = &osdDrainStrategy{
					mockKubeClient,
					&corev1.Node{},
					nodeDrainConfig,
					[]TimedDrainStrategy{},
				}
			})
			It("should not fail before default timeout has elapsed", func() {
				notLongEnough := &metav1.Time{Time: time.Now().Add(nodeDrainConfig.GetTimeOutDuration() / 2)}
				result, err := osdDrain.HasFailed(notLongEnough)
				Expect(result).To(BeFalse())
				Expect(err).To(BeNil())
			})
			It("should fail after default timeout has elapsed", func() {
				tooLongAgo := &metav1.Time{Time: time.Now().Add(-2 * nodeDrainConfig.GetTimeOutDuration())}
				result, err := osdDrain.HasFailed(tooLongAgo)
				Expect(result).To(BeTrue())
				Expect(err).To(BeNil())
			})
		})

		Context("OSD Time Based Drain Strategy failure", func() {
			BeforeEach(func() {
				mockCtrl = gomock.NewController(GinkgoT())
				mockKubeClient = mocks.NewMockClient(mockCtrl)
				mockTimedDrainOne = NewMockTimedDrainStrategy(mockCtrl)
				mockTimedDrainTwo = NewMockTimedDrainStrategy(mockCtrl)
				nodeDrainConfig = &NodeDrain{
					WorkerNodeTime: 8,
				}
				osdDrain = &osdDrainStrategy{
					mockKubeClient,
					&corev1.Node{},
					nodeDrainConfig,
					[]TimedDrainStrategy{mockTimedDrainTwo, mockTimedDrainOne},
				}
			})
			It("should fail after the last strategy has failed + allowed time for drain to occur", func() {
				drainStartedSixtyNineMinsAgo := &metav1.Time{Time: time.Now().Add(-69 * time.Minute)}
				mockOneDuration := time.Minute * 30
				mockTwoDuration := time.Minute * 60
				gomock.InOrder(
					// Need to use 'Any' as the sort function calls these functions many times
					mockTimedDrainOne.EXPECT().GetWaitDuration().Return(mockOneDuration).AnyTimes(),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration).AnyTimes(),
					mockTimedDrainTwo.EXPECT().GetStrategy().Return(mockStrategyTwo),
					mockStrategyTwo.EXPECT().HasFailed().Return(true, nil),
					mockTimedDrainTwo.EXPECT().GetWaitDuration().Return(mockTwoDuration),
				)
				result, err := osdDrain.HasFailed(drainStartedSixtyNineMinsAgo)
				Expect(result).To(BeTrue())
				Expect(err).To(BeNil())
			})
		})
	})
})

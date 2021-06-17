package drain

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Stuck Terminating Strategy", func() {

	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		sts            *stuckTerminatingStrategy
		node           *corev1.Node
		podList        corev1.PodList

		NODENAME string
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		NODENAME = "n1"

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: NODENAME,
			},
		}
		sts = &stuckTerminatingStrategy{
			client:  mockKubeClient,
			filters: []pod.PodPredicate{isOnNode(node), hasNoFinalizers, isTerminating},
		}

		podList = corev1.PodList{
			Items: []corev1.Pod{
				{

					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod1",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: corev1.PodSpec{
						NodeName: NODENAME,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod2",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						Finalizers: []string{
							"finalizer2",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: NODENAME,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod3",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: corev1.PodSpec{
						NodeName: "n2",
					},
				},
			},
		}
	})

	Context("Execute stuck terminating strategy on a node", func() {

		It("Successfully deletes pods stuck in terminating state", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := sts.Execute(node)
			Expect(result.HasExecuted).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Does nothing if no pod found in terminating state", func() {
			noTerminatingPods := corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "p1",
						},
						Spec: corev1.PodSpec{
							NodeName: NODENAME,
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "p2",
						},
						Spec: corev1.PodSpec{
							NodeName: NODENAME,
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, noTerminatingPods),
			)
			result, err := sts.Execute(node)
			Expect(result.HasExecuted).To(BeFalse())
			Expect(err).To(BeNil())
		})

		It("Returns error if fails to return a list of pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			_, err := sts.Execute(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})

		It("Returns error if failed to delete pod stuck in terminating state", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			_, err := sts.Execute(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Check if it's still valid to apply stuckTerminating strategy on a node", func() {
		It("Returns true if there are target pods stuck in terminating with no finalizers", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			valid, err := sts.IsValid(node)
			Expect(valid).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Returns false if there are any errors while getting list of pods stuck in terminating wtih no finalizers", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			valid, err := sts.IsValid(node)
			Expect(valid).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())

		})
	})

	Context("Get Pod List with no finalizers and stuck in terminating state", func() {
		It("Returns list of pods with no errors", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			_, err := sts.getPodList(node)
			Expect(err).To(BeNil())
		})

		It("Returns no pods if there is any error while listing pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			_, err := sts.getPodList(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})
})

package drain

import (
	"context"
	"fmt"

	"github.com/golang/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	NODENAME = "n1"
)

var _ = Describe("Remove Finalizer Strategy", func() {

	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		rfs            *removeFinalizersStrategy
		node           *corev1.Node
		podList        corev1.PodList
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: NODENAME,
			},
		}
		rfs = &removeFinalizersStrategy{
			client:  mockKubeClient,
			filters: []pod.PodPredicate{isOnNode(node), hasFinalizers},
		}

		podList = corev1.PodList{
			Items: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						Finalizers: []string{
							"finalizer1",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: NODENAME,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod2",
						Finalizers: []string{
							"finalizer2",
						},
					},
					Spec: corev1.PodSpec{
						NodeName: "dummy",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod3",
					},
					Spec: corev1.PodSpec{
						NodeName: NODENAME,
					},
				},
			},
		}
	})

	Context("Execute remove finalizers strategy on a node", func() {
		It("Successfully removes finalizers from pod with finalizer", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, pod *corev1.Pod) error {
						Expect(len(pod.ObjectMeta.Finalizers)).To(Equal(0))
						return nil
					}),
			)
			result, err := rfs.Execute(node)
			Expect(result.HasExecuted).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Does nothing if no pod found with finalizer", func() {
			noFinalizerPods := corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "dummy-pod",
						},
						Spec: corev1.PodSpec{
							NodeName: NODENAME,
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, noFinalizerPods),
			)
			result, err := rfs.Execute(node)
			Expect(result.HasExecuted).To(BeFalse())
			Expect(err).To(BeNil())
		})

		It("Returns error if fails to return a list of pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			_, err := rfs.Execute(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})

		It("Returns error if failed to remove finalizer from the pod", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			_, err := rfs.Execute(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Check if it's still valid to apply removeFinalizerStrategy on a node", func() {
		It("Returns true if there are target pods with finalizers", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			valid, err := rfs.IsValid(node)
			Expect(valid).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Returns false if there are any errors while getting list of pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			valid, err := rfs.IsValid(node)
			Expect(valid).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())

		})
	})

	Context("Get Pod List with finalizers", func() {
		It("Returns list of pods with no errors", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			_, err := rfs.getPodList(node)
			Expect(err).To(BeNil())
		})

		It("Returns no pods if there is any error while listing pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			_, err := rfs.getPodList(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})
})

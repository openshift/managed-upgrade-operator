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

var _ = Describe("Pod Delete Strategy", func() {

	var (
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		pds            *podDeletionStrategy
		node           *corev1.Node
		podList        corev1.PodList
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "n1",
			},
		}
		pds = &podDeletionStrategy{
			client:  mockKubeClient,
			filters: []pod.PodPredicate{isOnNode(node)},
		}

		podList = corev1.PodList{
			Items: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
					},
					Spec: corev1.PodSpec{
						NodeName: "n1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod2",
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: corev1.PodSpec{
						NodeName: "n1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod3",
					},
					Spec: corev1.PodSpec{
						NodeName: "n2",
					},
				},
			},
		}
	})

	Context("Execute pod delete strategy on a node", func() {

		It("Successfully deletes pods on a node", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := pds.Execute(node)
			Expect(result.HasExecuted).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Does nothing if pod already has deletion time stamp", func() {
			noDeletePods := corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "p1",
							DeletionTimestamp: &metav1.Time{Time: time.Now()},
						},
						Spec: corev1.PodSpec{
							NodeName: "n1",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "p2",
							DeletionTimestamp: &metav1.Time{Time: time.Now()},
						},
						Spec: corev1.PodSpec{
							NodeName: "n2",
						},
					},
				},
			}
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, noDeletePods),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(3),
			)
			result, err := pds.Execute(node)
			Expect(result.HasExecuted).To(BeFalse())
			Expect(err).To(BeNil())
		})

		It("Returns error if fails to return a list of pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			_, err := pds.Execute(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})

		It("Returns error if failed to delete pod", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			_, err := pds.Execute(node)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Check if it's still valid to delete a pod", func() {
		It("Returns true if there are target pods to be deleted", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			valid, err := pds.IsValid(node)
			Expect(valid).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Returns false if there are any errors while getting list of pods to be deleted", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			valid, err := pds.IsValid(node)
			Expect(valid).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())

		})
	})
})

package pod

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

var _ = Describe("Pod Filter", func() {

	var (
		logger         logr.Logger
		podList        *corev1.PodList
		mockKubeClient *mocks.MockClient
		mockCtrl       *gomock.Controller
		passPredicate  PodPredicate = func(p corev1.Pod) bool {
			return true
		}
		failPredicate PodPredicate = func(p corev1.Pod) bool {
			return false
		}
		NODENAME = "n1"

		node = &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: NODENAME,
			},
		}
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		podList = &corev1.PodList{
			Items: []corev1.Pod{
				{}, {}, {},
			},
		}
		logger = logf.Log.WithName("pod utility test logger")
	})

	Context("Filtering", func() {
		It("should return pods that match a predicate", func() {
			filteredPods := FilterPods(podList, passPredicate)
			Expect(len(filteredPods.Items)).To(Equal(len(podList.Items)))
		})
		It("should return pods that matches all predicates", func() {
			filteredPods := FilterPods(podList, passPredicate, passPredicate)
			Expect(len(filteredPods.Items)).To(Equal(len(podList.Items)))
		})
		It("should filter pods that do not match the predicate(s)", func() {
			filteredPods := FilterPods(podList, passPredicate, failPredicate, passPredicate)
			Expect(len(filteredPods.Items)).To(Equal(0))
		})
	})

	Context("Listing", func() {
		It("should return pods that match a predicate", func() {
			mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *podList)
			filters := []PodPredicate{passPredicate}
			podList, err := GetPodList(mockKubeClient, node, filters)
			Expect(len(podList.Items)).To(Equal(len(podList.Items)))
			Expect(err).To(BeNil())
		})
		It("should return pods that matches all predicates", func() {
			mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *podList)
			filters := []PodPredicate{passPredicate, passPredicate}
			podList, err := GetPodList(mockKubeClient, node, filters)
			Expect(len(podList.Items)).To(Equal(len(podList.Items)))
			Expect(err).To(BeNil())
		})
		It("should return no pods that do not match the predicate", func() {
			mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *podList)
			filters := []PodPredicate{failPredicate}
			podList, err := GetPodList(mockKubeClient, node, filters)
			Expect(len(podList.Items)).To(Equal(0))
			Expect(err).To(BeNil())
		})
		It("Returns no pods if there is any error while listing pods", func() {
			mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, *podList).Return(fmt.Errorf("fake error"))
			filters := []PodPredicate{failPredicate}
			_, err := GetPodList(mockKubeClient, node, filters)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Removing Finalizers", func() {
		var (
			podList *corev1.PodList
		)

		BeforeEach(func() {
			podList = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "testpod",
							Finalizers: []string{
								"deleteThisFinalizer",
								"deleteThisFinalizerAlso",
							},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "testpod3",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "testpod",
							Finalizers: []string{
								"deleteThisFinalizer",
							},
						},
					},
				},
			}
		})

		It("Should remove finalizers if they exist", func() {
			mockKubeClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(2)
			result, err := RemoveFinalizersFromPod(mockKubeClient, logger, podList)
			Expect(err).To(BeNil())
			Expect(result.NumRemoved).To(Equal(2))
		})
	})

	Context("Deleting Pods", func() {
		var (
			podList *corev1.PodList
		)

		BeforeEach(func() {
			podList = &corev1.PodList{
				Items: []corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "testpodBeingDeleted",
							DeletionTimestamp: &metav1.Time{Time: time.Now()},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:              "testpodBeingDeletedToo",
							DeletionTimestamp: &metav1.Time{Time: time.Now()},
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "testpod3",
						},
					},
				},
			}
		})

		Context("When deleting pods that aren't already deleting", func() {
			It("Should not attempt to re-delete deleting pods", func() {
				gp := int64(0)
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)
				result, err := DeletePods(mockKubeClient, logger, podList, true, &client.DeleteOptions{GracePeriodSeconds: &gp})
				Expect(err).To(BeNil())
				Expect(result.NumMarkedForDeletion).To(Equal(1))
			})
			It("Should attempt to re-delete deleting pods if asked", func() {
				gp := int64(0)
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(3)
				result, err := DeletePods(mockKubeClient, logger, podList, false, &client.DeleteOptions{GracePeriodSeconds: &gp})
				Expect(err).To(BeNil())
				Expect(result.NumMarkedForDeletion).To(Equal(3))
			})

		})
	})

})

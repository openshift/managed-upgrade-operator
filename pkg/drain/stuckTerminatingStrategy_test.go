package drain

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
	"github.com/openshift/managed-upgrade-operator/util/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Stuck Terminating Strategy", func() {

	const (
		POD_NAMESPACE = "test-namespace"
	)

	var (
		logger         logr.Logger
		mockCtrl       *gomock.Controller
		mockKubeClient *mocks.MockClient
		sts            *stuckTerminatingStrategy
		node           *corev1.Node
		podList        corev1.PodList
		nsPredicate    pod.PodPredicate

		NODENAME string
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
		logger = logf.Log.WithName("stuck terminating strategy test logger")
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
		nsPredicate = isAllowedNamespace([]string{POD_NAMESPACE})

		podList = corev1.PodList{
			Items: []corev1.Pod{
				{

					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod1",
						Namespace:         POD_NAMESPACE,
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
					Spec: corev1.PodSpec{
						NodeName: NODENAME,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pod2",
						Namespace:         POD_NAMESPACE,
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
						Namespace:         POD_NAMESPACE,
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
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()),
			)
			result, err := sts.Execute(node, logger)
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
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, noTerminatingPods),
			)
			result, err := sts.Execute(node, logger)
			Expect(result.HasExecuted).To(BeFalse())
			Expect(err).To(BeNil())
		})

		It("Returns error if fails to return a list of pods", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			_, err := sts.Execute(node, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})

		It("Returns error if failed to delete pod stuck in terminating state", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
				mockKubeClient.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			_, err := sts.Execute(node, logger)
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())
		})
	})

	Context("Check if it's still valid to apply stuckTerminating strategy on a node", func() {
		It("Returns true if there are target pods stuck in terminating with no finalizers", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			valid, err := sts.IsValid(node, logger)
			Expect(valid).To(BeTrue())
			Expect(err).To(BeNil())
		})

		It("Returns false if there are any errors while getting list of pods stuck in terminating with no finalizers", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList).Return(fmt.Errorf("fake error")),
			)
			valid, err := sts.IsValid(node, logger)
			Expect(valid).To(BeFalse())
			Expect(err).To(HaveOccurred())
			Expect(err).NotTo(BeNil())

		})
	})

	Context("Ensure that pods with namespaces are respected based on config preferences", func() {
		It("Will not consider a pod if the pod's namespace should be ignored", func() {
			nsPredicate = isAllowedNamespace([]string{POD_NAMESPACE})
			sts.filters = append(sts.filters, nsPredicate)

			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			valid, err := sts.IsValid(node, logger)
			Expect(valid).To(BeFalse())
			Expect(err).To(BeNil())
		})

		It("Will consider a pod if its namespace isn't in the ignore list", func() {
			nsPredicate = isAllowedNamespace([]string{})
			sts.filters = append(sts.filters, nsPredicate)
			gomock.InOrder(
				mockKubeClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(1, podList),
			)
			valid, err := sts.IsValid(node, logger)
			Expect(valid).To(BeTrue())
			Expect(err).To(BeNil())
		})
	})
})

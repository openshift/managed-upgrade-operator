package util

import (
	"fmt"
	"github.com/golang/mock/gomock"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/managed-upgrade-operator/util/mocks"
)

var _ = Describe("Access token tests", func() {

	var (
		// mocks
		mockKubeClient *mocks.MockClient
		mockCtrl       *gomock.Controller
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockKubeClient = mocks.NewMockClient(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when fetching the cluster pull secret fails", func() {
		It("returns an error", func() {
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()),
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).Return(fmt.Errorf("fake error")),
			)
			_, err := GetAccessToken(mockKubeClient)
			Expect(err).ToNot(BeNil())
		})
	})

	Context("when the cluster pull secret doesn't have the expected key", func() {
		It("returns an error", func() {
			pullSecret := &corev1.Secret{Data: map[string][]byte{
				"thiswontwork": []byte("test"),
			}}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()),
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).SetArg(2, *pullSecret).Return(nil),
			)
			_, err := GetAccessToken(mockKubeClient)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("missing required key"))
		})
	})

	Context("when the decoded cluster pull secret isn't valid json", func() {
		It("returns an error", func() {
			pullSecret := &corev1.Secret{Data: map[string][]byte{
				".dockerconfigjson": []byte("not valid json at all"),
			}}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()),
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).SetArg(2, *pullSecret).Return(nil),
			)
			_, err := GetAccessToken(mockKubeClient)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("unable to interpret decoded pull secret as json"))
		})
	})

	Context("when the decoded pull secret doesn't have the expected json structure", func() {
		It("returns an error", func() {
			pullSecret := &corev1.Secret{Data: map[string][]byte{
				".dockerconfigjson": []byte(`{ "valid": "json", "but": ["not what we want"] }`),
			}}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()),
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).SetArg(2, *pullSecret).Return(nil),
			)
			_, err := GetAccessToken(mockKubeClient)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("unable to find auths section"))
		})
	})

	Context("when the pull secret can be fetched and decoded", func() {
		It("returns the pull secret", func() {
			pullSecret := &corev1.Secret{Data: map[string][]byte{
				".dockerconfigjson": []byte(`{ "auths": { "cloud.openshift.com": { "auth": "access token here" } } }`),
			}}
			gomock.InOrder(
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "version"}, gomock.Any()),
				mockKubeClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, gomock.Any()).SetArg(2, *pullSecret).Return(nil),
			)
			tok, err := GetAccessToken(mockKubeClient)
			Expect(err).To(BeNil())
			Expect(tok.PullSecret).To(Equal("access token here"))
		})
	})
})

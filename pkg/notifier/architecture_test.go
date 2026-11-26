package notifier

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	sdk "github.com/openshift-online/ocm-sdk-go"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	mockocm "github.com/openshift/managed-upgrade-operator/pkg/ocm/mocks"
	mockmanager "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	"go.uber.org/mock/gomock"
)

var _ = Describe("OCM notification policy architecture", func() {
	It("distinguishes policies with the same version and schedule by architecture", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/token" {
				_, _ = fmt.Fprint(w, `{"access_token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IlRlc3QgVXNlciIsImlhdCI6MTUxNjIzOTAyMn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c","token_type":"Bearer","expires_in":3600}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"total":2,"size":2,"items":[
    {"id":"migration-policy","version":"4.18.1","next_run":"2026-11-24T12:00:00Z","architecture":"Multi"},
    {"id":"version-policy","version":"4.18.1","next_run":"2026-11-24T12:00:00Z"}
   ]}`)
		}))
		defer server.Close()
		conn, err := sdk.NewConnectionBuilder().URL(server.URL).TokenURL(server.URL + "/token").Tokens("test-token").Build()
		Expect(err).NotTo(HaveOccurred())
		defer conn.Close()
		policies, err := conn.ClustersMgmt().V1().Clusters().Cluster("cluster").UpgradePolicies().List().Send()
		Expect(err).NotTo(HaveOccurred())

		for _, tc := range []struct {
			architecture configv1.ClusterVersionArchitecture
			wantID       string
		}{{"", "version-policy"}, {configv1.ClusterVersionArchitectureMulti, "migration-policy"}, {"unknown", ""}} {
			ctrl := gomock.NewController(GinkgoT())
			ocmClient := mockocm.NewMockOcmClient(ctrl)
			manager := mockmanager.NewMockUpgradeConfigManager(ctrl)
			uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{Version: "4.18.1", Architecture: tc.architecture}, UpgradeAt: "2026-11-24T12:00:00Z",
			}}
			manager.EXPECT().Get().Return(uc, nil)
			ocmClient.EXPECT().GetClusterUpgradePolicies("cluster").Return(policies, nil)
			notifier := &ocmNotifier{ocmClient: ocmClient, upgradeConfigManager: manager}
			id, err := notifier.getPolicyIdForUpgradeConfig("cluster")
			if tc.wantID == "" {
				Expect(err).To(MatchError("no policy matches the current UpgradeConfig"))
				Expect(id).To(BeNil())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(id).NotTo(BeNil())
				Expect(*id).To(Equal(tc.wantID))
			}
			ctrl.Finish()
		}
	})
})

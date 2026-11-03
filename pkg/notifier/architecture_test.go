package notifier

import (
	"io"
	"net/http"
	"strings"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	mockocm "github.com/openshift/managed-upgrade-operator/pkg/ocm/mocks"
	mockmanager "github.com/openshift/managed-upgrade-operator/pkg/upgradeconfigmanager/mocks"
	"go.uber.org/mock/gomock"
)

func TestNotificationMatchesPolicyArchitecture(t *testing.T) {
	for _, tc := range []struct {
		architecture configv1.ClusterVersionArchitecture
		wantID       string
	}{
		{"", "version-policy"},
		{"Multi", "migration-policy"},
	} {
		t.Run(tc.wantID, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ocmClient := mockocm.NewMockOcmClient(ctrl)
			manager := mockmanager.NewMockUpgradeConfigManager(ctrl)
			uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{
				Desired: upgradev1alpha1.Update{Version: "4.18.1", Architecture: tc.architecture}, UpgradeAt: "2026-09-29T12:00:00Z",
			}}
			policies, err := cmv1.NewUpgradePoliciesClient(architecturePolicyTransport(`{"total":2,"items":[
				{"id":"version-policy","version":"4.18.1","next_run":"2026-09-29T12:00:00Z"},
				{"id":"migration-policy","version":"4.18.1","next_run":"2026-09-29T12:00:00Z","architecture":"multi"}
			]}`), "/upgrade_policies").List().Send()
			if err != nil {
				t.Fatal(err)
			}
			manager.EXPECT().Get().Return(uc, nil)
			ocmClient.EXPECT().GetClusterUpgradePolicies("cluster").Return(policies, nil)
			notifier := &ocmNotifier{ocmClient: ocmClient, upgradeConfigManager: manager}
			id, err := notifier.getPolicyIdForUpgradeConfig("cluster")
			if err != nil || id == nil || *id != tc.wantID {
				t.Fatalf("wrong policy selected: %v, %v", id, err)
			}
		})
	}
}

// Return a wire response through the SDK decoder, including the architecture field.
type architecturePolicyTransport string

func (data architecturePolicyTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}, nil
}

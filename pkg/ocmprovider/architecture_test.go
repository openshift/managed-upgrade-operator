package ocmprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	mockocm "github.com/openshift/managed-upgrade-operator/pkg/ocm/mocks"
	"go.uber.org/mock/gomock"
)

func TestProviderArchitecturePolicies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		architecture string
		version      string
		state        string
		want         *upgradev1alpha1.Update
		wantError    bool
	}{
		{"version upgrade", "", "4.18.2", "scheduled", &upgradev1alpha1.Update{Version: "4.18.2", Channel: "fast-4.18"}, false},
		{"migration", "Multi", "4.18.1", "scheduled", &upgradev1alpha1.Update{Version: "4.18.1", Architecture: "Multi"}, false},
		{"lowercase migration", "multi", "4.18.1", "scheduled", &upgradev1alpha1.Update{Version: "4.18.1", Architecture: "Multi"}, false},
		{"unsupported architecture", "amd64", "4.18.1", "scheduled", nil, true},
		{"unscheduled migration", "Multi", "4.18.1", "pending", nil, false},
		{"completed migration", "Multi", "4.18.1", "completed", nil, false},
		{"cancelled migration", "Multi", "4.18.1", "cancelled", nil, false},
		{"missing version", "Multi", "", "scheduled", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ocmClient := mockocm.NewMockOcmClient(ctrl)
			cluster, err := cmv1.NewCluster().ID("cluster").
				Version(cmv1.NewVersion().ID("4.18.1").ChannelGroup("fast")).
				NodeDrainGracePeriod(cmv1.NewValue().Value(60)).Build()
			if err != nil {
				t.Fatal(err)
			}
			var policies ocm.UpgradePolicyList
			data := fmt.Sprintf(`{"total":1,"items":[{"id":"policy","version":%q,"architecture":%q,"next_run":"2026-09-29T12:00:00Z","schedule_type":"manual","upgrade_type":"OSD"}]}`, tc.version, tc.architecture)
			if err := json.Unmarshal([]byte(data), &policies); err != nil {
				t.Fatal(err)
			}
			state, err := cmv1.NewUpgradePolicyState().Value(cmv1.UpgradePolicyStateValue(tc.state)).Build()
			if err != nil {
				t.Fatal(err)
			}
			ocmClient.EXPECT().GetCluster().Return(cluster, nil)
			ocmClient.EXPECT().GetClusterUpgradePolicies("cluster").Return(&policies, nil)
			ocmClient.EXPECT().GetClusterUpgradePolicyState("policy", "cluster").Return(state, nil)
			provider := &ocmProvider{ocmClient: ocmClient, upgradeType: upgradev1alpha1.OSD}
			specs, err := provider.Get()
			if tc.wantError {
				if !errors.Is(err, ErrProcessingPolicies) {
					t.Fatalf("expected policy processing error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if len(specs) != 0 {
					t.Fatalf("inactive policy produced specs: %+v", specs)
				}
				return
			}
			if len(specs) != 1 || specs[0].Desired != *tc.want {
				t.Fatalf("unexpected specs: %+v, want desired %+v", specs, tc.want)
			}
			if specs[0].UpgradeAt != "2026-09-29T12:00:00Z" || specs[0].PDBForceDrainTimeout != 60 || specs[0].Type != upgradev1alpha1.OSD || !specs[0].CapacityReservation {
				t.Fatalf("policy settings were lost: %+v", specs[0])
			}
			// Migration must preserve the cluster channel even if OCM's inferred channel differs.
			cv := &configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{Channel: "stable-4.18"}, Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1"}}}
			if err := specs[0].Desired.ValidateArchitecture(cv); err != nil {
				t.Fatalf("generated request failed architecture validation: %v", err)
			}
		})
	}
}

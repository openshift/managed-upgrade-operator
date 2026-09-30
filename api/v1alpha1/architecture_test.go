package v1alpha1

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestArchitectureHistory(t *testing.T) {
	histories := UpgradeHistories{{Version: "4.18.1", Phase: UpgradePhaseUpgraded}}
	desired := Update{Version: "4.18.1", Architecture: "Multi"}
	if histories.GetHistoryForUpdate(desired) != nil {
		t.Fatal("version upgrade mistaken for migration")
	}
	histories.SetHistory(UpgradeHistory{Version: desired.Version, Architecture: desired.Architecture, Phase: UpgradePhaseNew})
	history := histories.GetHistoryForUpdate(desired)
	if history == nil || history.Phase != UpgradePhaseNew || len(histories) != 2 {
		t.Fatalf("missing migration history: %+v", histories)
	}
	history.Phase = UpgradePhaseUpgraded
	histories.SetHistory(*history)
	if len(histories) != 2 || histories.GetHistoryForUpdate(desired).Phase != UpgradePhaseUpgraded {
		t.Fatal("migration history not updated")
	}
	if histories.GetHistoryForUpdate(Update{Version: "4.18.1"}).Phase != UpgradePhaseUpgraded {
		t.Fatal("version history changed")
	}
}

func TestValidateArchitecture(t *testing.T) {
	cv := &configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{Channel: "stable-4.18"}, Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1"}}}
	for _, tc := range []struct {
		name    string
		desired Update
		valid   bool
	}{
		{"omitted", Update{}, true},
		{"image upgrade", Update{Image: "release-image"}, true},
		{"migration", Update{Architecture: "Multi", Version: "4.18.1"}, true},
		{"same channel", Update{Architecture: "Multi", Version: "4.18.1", Channel: "stable-4.18"}, true},
		{"different channel", Update{Architecture: "Multi", Version: "4.18.1", Channel: "fast-4.18"}, false},
		{"version upgrade changes channel", Update{Version: "4.19.1", Channel: "stable-4.19"}, true},
		{"unsupported architecture", Update{Architecture: "amd64", Version: "4.18.1"}, false},
		{"missing version", Update{Architecture: "Multi"}, false},
		{"conflicting image", Update{Architecture: "Multi", Version: "4.18.1", Image: "release-image"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.desired.ValidateArchitecture(cv)
			if (err == nil) != tc.valid {
				t.Fatalf("valid = %v, want %v; error: %v", err == nil, tc.valid, err)
			}
		})
	}
}

func TestValidateArchitectureMigrationVersion(t *testing.T) {
	for _, arch := range []configv1.ClusterVersionArchitecture{"", "amd64", configv1.ClusterVersionArchitectureMulti} {
		for _, version := range []string{"4.18.0", "4.18.1", "4.18.2"} {
			t.Run(string(arch)+"/"+version, func(t *testing.T) {
				cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1", Architecture: arch}}}
				desired := Update{Architecture: "Multi", Version: version}
				err := desired.ValidateArchitecture(cv)
				valid := arch == configv1.ClusterVersionArchitectureMulti || version == "4.18.1"
				if (err == nil) != valid {
					t.Fatalf("valid = %v, want %v; error: %v", err == nil, valid, err)
				}
			})
		}
	}
}

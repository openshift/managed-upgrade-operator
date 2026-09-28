package v1alpha1

import "testing"

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
	for _, tc := range []struct {
		name    string
		desired Update
		valid   bool
	}{
		{"omitted", Update{}, true},
		{"image upgrade", Update{Image: "release-image"}, true},
		{"migration", Update{Architecture: "Multi", Version: "4.18.1"}, true},
		{"unsupported architecture", Update{Architecture: "amd64", Version: "4.18.1"}, false},
		{"missing version", Update{Architecture: "Multi"}, false},
		{"conflicting image", Update{Architecture: "Multi", Version: "4.18.1", Image: "release-image"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.desired.ValidateArchitecture()
			if (err == nil) != tc.valid {
				t.Fatalf("valid = %v, want %v; error: %v", err == nil, tc.valid, err)
			}
		})
	}
}

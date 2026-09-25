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

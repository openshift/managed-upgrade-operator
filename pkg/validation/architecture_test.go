package validation

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
)

func TestArchitectureMigrationValidation(t *testing.T) {
	cv := &configv1.ClusterVersion{Spec: configv1.ClusterVersionSpec{Channel: "stable-4.18"}, Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1"}, History: []configv1.UpdateHistory{{Version: "4.18.1", Image: "current", State: configv1.CompletedUpdate, CompletionTime: &metav1.Time{Time: time.Now()}}}, AvailableUpdates: []configv1.Release{{Version: "4.18.2", Image: "next"}}}}
	for _, tc := range []struct {
		name    string
		desired upgradev1alpha1.Update
		valid   bool
	}{
		{"same version without graph edge", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.1"}, true},
		{"same channel", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.1", Channel: "stable-4.18"}, true},
		{"different available version", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.2", Channel: "stable-4.18"}, true},
		{"unavailable target", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.3", Channel: "stable-4.18"}, false},
		{"downgrade", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.0", Channel: "stable-4.18"}, false},
		{"different version and channel", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.2", Channel: "fast-4.18"}, true},
		{"invalid version", upgradev1alpha1.Update{Architecture: "Multi", Version: "invalid"}, false},
		{"missing version", upgradev1alpha1.Update{Architecture: "Multi"}, false},
		{"different version without channel", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.2"}, true},
		{"image conflict", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.1", Image: "invalid"}, false},
		{"different channel", upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.1", Channel: "stable-4.19"}, true},
		{"invalid architecture", upgradev1alpha1.Update{Architecture: "amd64", Version: "4.18.1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{UpgradeAt: "2026-09-24T12:00:00Z", Desired: tc.desired}}
			result, err := (&validator{Cincinnati: true}).IsValidUpgradeConfig(nil, uc, cv, logr.Discard())
			if err != nil || result.IsValid != tc.valid || result.IsAvailableUpdate != tc.valid {
				t.Fatalf("unexpected result: %+v, %v", result, err)
			}
		})
	}
}

func TestArchitectureMigrationRejectsAmbiguousCVOPayload(t *testing.T) {
	cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{AvailableUpdates: []configv1.Release{
		{Version: "4.18.2", Image: "first"}, {Version: "4.18.2", Image: "second"},
	}}}
	uc := &upgradev1alpha1.UpgradeConfig{
		ObjectMeta: metav1.ObjectMeta{Namespace: "openshift-managed-upgrade-operator", Name: "managed-upgrade-config"},
		Spec: upgradev1alpha1.UpgradeConfigSpec{
			UpgradeAt: "2026-09-24T12:00:00Z", Desired: upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.2"},
		},
	}
	result, err := (&validator{}).IsValidUpgradeConfig(nil, uc, cv, logr.Discard())
	if err != nil || result.IsValid || result.IsAvailableUpdate {
		t.Fatalf("ambiguous payload accepted: %+v, %v", result, err)
	}
	for _, context := range []string{
		"UpgradeConfig openshift-managed-upgrade-operator/managed-upgrade-config: CVO validation failed:",
		"spec.desiredUpdate.version",
		"4.18.2",
		"there are multiple possible payloads for this version",
	} {
		if !strings.Contains(result.Message, context) {
			t.Errorf("validation message %q missing context %q", result.Message, context)
		}
	}
}

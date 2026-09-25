package clusterversion

import (
	"context"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestArchitectureMigration(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := configv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, initial := range []*configv1.Update{nil, {Version: "4.18.1", Image: "single-arch-image", Force: true}} {
		cv := &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{Name: OSD_CV_NAME},
			Spec:       configv1.ClusterVersionSpec{Channel: "stable-4.18", DesiredUpdate: initial},
			Status:     configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1", Image: "single-arch-image"}, History: []configv1.UpdateHistory{{Version: "4.18.1", Image: "single-arch-image", State: configv1.CompletedUpdate}}},
		}
		kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cv).Build()
		c := NewCVClient(kube)
		uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{Desired: upgradev1alpha1.Update{Version: "4.18.1", Architecture: configv1.ClusterVersionArchitectureMulti}}}
		if started, err := c.HasUpgradeCommenced(uc); err != nil || started {
			t.Fatalf("migration prematurely commenced: %v, %v", started, err)
		}
		for i := 0; i < 2; i++ {
			if done, err := c.EnsureDesiredConfig(uc); err != nil || !done {
				t.Fatalf("request failed: %v, %v", done, err)
			}
		}
		if err := kube.Get(context.Background(), types.NamespacedName{Name: OSD_CV_NAME}, cv); err != nil {
			t.Fatal(err)
		}
		want := configv1.Update{Version: "4.18.1", Architecture: configv1.ClusterVersionArchitectureMulti}
		if cv.Spec.DesiredUpdate == nil || !reflect.DeepEqual(*cv.Spec.DesiredUpdate, want) || cv.Spec.Channel != "stable-4.18" {
			t.Fatalf("unexpected spec: %+v", cv.Spec)
		}
		if started, err := c.HasUpgradeCommenced(uc); err != nil || !started {
			t.Fatalf("migration not recognized: %v, %v", started, err)
		}
		// A different version and channel are passed through for CVO to resolve.
		uc.Spec.Desired.Version = "4.18.2"
		uc.Spec.Desired.Channel = "fast-4.18"
		if done, err := c.EnsureDesiredConfig(uc); err != nil || !done {
			t.Fatalf("request not passed to CVO: %v, %v", done, err)
		}
		if err := kube.Get(context.Background(), types.NamespacedName{Name: OSD_CV_NAME}, cv); err != nil {
			t.Fatal(err)
		}
		want.Version = "4.18.2"
		if !reflect.DeepEqual(*cv.Spec.DesiredUpdate, want) || cv.Spec.Channel != "fast-4.18" {
			t.Fatalf("requested version/channel not preserved: %+v", cv.Spec)
		}
		if started, err := c.HasUpgradeCommenced(uc); err != nil || !started {
			t.Fatalf("request not recognized: %v, %v", started, err)
		}
		// A channel-only change must not be mistaken for an already submitted request.
		uc.Spec.Desired.Channel = "stable-4.18"
		if started, err := c.HasUpgradeCommenced(uc); err != nil || started {
			t.Fatalf("channel change ignored: %v, %v", started, err)
		}
		if done, err := c.EnsureDesiredConfig(uc); err != nil || !done {
			t.Fatalf("channel change failed: %v, %v", done, err)
		}
		if err := kube.Get(context.Background(), types.NamespacedName{Name: OSD_CV_NAME}, cv); err != nil {
			t.Fatal(err)
		}
		if cv.Spec.Channel != "stable-4.18" {
			t.Fatalf("channel not updated: %s", cv.Spec.Channel)
		}
	}
}

func TestArchitectureMigrationCompletion(t *testing.T) {
	uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{Desired: upgradev1alpha1.Update{Version: "4.18.1", Architecture: configv1.ClusterVersionArchitectureMulti}}}
	old := configv1.UpdateHistory{Version: "4.18.1", Image: "single", State: configv1.CompletedUpdate}
	for _, tc := range []struct {
		name         string
		architecture configv1.ClusterVersionArchitecture
		history      []configv1.UpdateHistory
		completed    bool
	}{
		{"old payload", "", []configv1.UpdateHistory{old}, false},
		{"status updated before history", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory{old}, false},
		{"no history", configv1.ClusterVersionArchitectureMulti, nil, false},
		{"migration in progress", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory{{Version: "4.18.1", Image: "multi", State: configv1.PartialUpdate}, old}, false},
		{"migration completed", configv1.ClusterVersionArchitectureMulti, []configv1.UpdateHistory{{Version: "4.18.1", Image: "multi", State: configv1.CompletedUpdate}, old}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1", Image: "multi", Architecture: tc.architecture}, History: tc.history}}
			if got := (&clusterVersionClient{}).HasUpgradeCompleted(cv, uc); got != tc.completed {
				t.Fatalf("completed = %v, want %v", got, tc.completed)
			}
		})
	}
}

func TestArchitectureMigrationWaitsForExistingRollout(t *testing.T) {
	uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{Desired: upgradev1alpha1.Update{Version: "4.18.1", Architecture: "Multi"}}}
	for _, history := range [][]configv1.UpdateHistory{nil, {{Version: "4.18.1", State: configv1.PartialUpdate}}} {
		cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{Desired: configv1.Release{Version: "4.18.1"}, History: history}}
		if done, err := (&clusterVersionClient{}).runUpgradeWithArchitecture(cv, uc); err != nil || done {
			t.Fatalf("migration did not wait for existing rollout: %v, %v", done, err)
		}
	}
}

func TestArchitectureMigrationWaitsForRequestedVersion(t *testing.T) {
	uc := &upgradev1alpha1.UpgradeConfig{Spec: upgradev1alpha1.UpgradeConfigSpec{Desired: upgradev1alpha1.Update{Architecture: "Multi", Version: "4.18.2"}}}
	cv := &configv1.ClusterVersion{Status: configv1.ClusterVersionStatus{
		Desired: configv1.Release{Version: "4.18.1", Architecture: "Multi", Image: "multi-current"},
		History: []configv1.UpdateHistory{{Version: "4.18.1", Image: "multi-current", State: configv1.CompletedUpdate}},
	}}
	c := &clusterVersionClient{}
	if c.HasUpgradeCompleted(cv, uc) {
		t.Fatal("migration at current version completed a different-version request")
	}
	cv.Status.Desired = configv1.Release{Version: "4.18.2", Architecture: "Multi", Image: "multi-target"}
	cv.Status.History = append([]configv1.UpdateHistory{{Version: "4.18.2", Image: "multi-target", State: configv1.PartialUpdate}}, cv.Status.History...)
	if c.HasUpgradeCompleted(cv, uc) {
		t.Fatal("incomplete target payload reported complete")
	}
	cv.Status.History[0].State = configv1.CompletedUpdate
	if !c.HasUpgradeCompleted(cv, uc) {
		t.Fatal("completed target payload not recognized")
	}
}

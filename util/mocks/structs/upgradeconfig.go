package structs

import (
	"fmt"
	"time"

	api "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type testUpgradeConfigBuilder struct {
	uc api.UpgradeConfig
}

func (t *testUpgradeConfigBuilder) GetUpgradeConfig() *api.UpgradeConfig {
	return &t.uc
}

func NewUpgradeConfigBuilder() *testUpgradeConfigBuilder {
	var testTime time.Time
	testTime, _ = time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
	return &testUpgradeConfigBuilder{
		uc: api.UpgradeConfig{

			ObjectMeta: metav1.ObjectMeta{
				Name:      "fakeUpgradeConfig",
				Namespace: "fakeNamespace",
			},
			Spec: api.UpgradeConfigSpec{
				Type: api.OSD,
				Desired: api.Update{
					Version: "fakeVersion",
					Channel: "fakeChannel",
				},
				UpgradeAt:            testTime.Format(time.RFC3339),
				PDBForceDrainTimeout: 60,
			},
			Status: api.UpgradeConfigStatus{
				History: []api.UpgradeHistory{
					{
						Version:            "fakeVersion",
						Phase:              api.UpgradePhaseUpgrading,
						StartTime:          &metav1.Time{Time: testTime},
						CompleteTime:       &metav1.Time{Time: testTime},
						WorkerStartTime:    &metav1.Time{Time: testTime},
						WorkerCompleteTime: &metav1.Time{Time: testTime},
						HealthCheck: api.HealthCheck{
							Failed: false,
							State:  "pre_upgrade",
						},
						Scaling: api.Scaling{
							Failed:    false,
							Dimension: "down",
						},
						ClusterVerificationFailed: false,
						ControlPlaneTimeout:       false,
						WorkerTimeout:             false,
						NodeDrain: api.Drain{
							Failed: false,
							Name:   "cool_node",
						},
						WindowBreached: false,
					},
				},
			},
		},
	}
}

func (t *testUpgradeConfigBuilder) WithNamespacedName(namespacedName types.NamespacedName) *testUpgradeConfigBuilder {
	t.uc.ObjectMeta.Name = namespacedName.Name
	t.uc.ObjectMeta.Namespace = namespacedName.Namespace
	return t
}

func (t *testUpgradeConfigBuilder) WithPhase(phase api.UpgradePhase) *testUpgradeConfigBuilder {
	t.uc.Status.History = []api.UpgradeHistory{
		api.UpgradeHistory{
			Version: t.uc.Spec.Desired.Version,
			Phase:   phase,
		},
	}
	return t
}

type UpgradeConfigMatcher struct {
	ActualUpgradeConfig api.UpgradeConfig
	FailReason          string
}

func NewUpgradeConfigMatcher() *UpgradeConfigMatcher {
	return &UpgradeConfigMatcher{}
}

func (m *UpgradeConfigMatcher) Matches(x interface{}) bool {
	ref, isCorrectType := x.(*api.UpgradeConfig)
	if !isCorrectType {
		m.FailReason = fmt.Sprintf("Unexpected type passed: want '%T', got '%T'", api.UpgradeConfig{}, x)
		return false
	}
	m.ActualUpgradeConfig = *ref.DeepCopy()
	return true
}

func (m *UpgradeConfigMatcher) String() string {
	return "Fail reason: " + m.FailReason
}

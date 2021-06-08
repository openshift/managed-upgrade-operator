package upgradeconfig

import (
	"reflect"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// StatusChangedPredicate is a function that executes predicates for an UpgradeConfig
var StatusChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.MetaOld == nil {
			log.Error(nil, "Update event has no old metadata", "event", e)
			return false
		}
		if e.ObjectOld == nil {
			log.Error(nil, "Update event has no old runtime object to update", "event", e)
			return false
		}
		if e.ObjectNew == nil {
			log.Error(nil, "Update event has no new runtime object for update", "event", e)
			return false
		}
		if e.MetaNew == nil {
			log.Error(nil, "Update event has no new metadata", "event", e)
			return false
		}
		newUp := e.ObjectNew.(*upgradev1alpha1.UpgradeConfig)
		oldUp := e.ObjectOld.(*upgradev1alpha1.UpgradeConfig)
		return (reflect.DeepEqual(newUp.Status, oldUp.Status))
	},
}

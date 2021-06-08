package nodekeeper

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
)

// IgnoreMasterPredicate holds predicate funcs
var IgnoreMasterPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		newNode, ok := e.MetaNew.(*corev1.Node)
		if !ok {
			return false
		}
		nodeLabels := newNode.GetLabels()
		return !hasMasterLabel(nodeLabels)
	},
	// Create is required to avoid reconciliation at controller initialisation.
	CreateFunc: func(e event.CreateEvent) bool {
		nodeLabels := e.Meta.GetLabels()
		return !hasMasterLabel(nodeLabels)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		nodeLabels := e.Meta.GetLabels()
		return !hasMasterLabel(nodeLabels)
	},
	GenericFunc: func(e event.GenericEvent) bool {
		nodeLabels := e.Meta.GetLabels()
		return !hasMasterLabel(nodeLabels)
	},
}

func hasMasterLabel(nodeLabels map[string]string) bool {
	_, ok := nodeLabels[machinery.MasterLabel]
	return ok
}

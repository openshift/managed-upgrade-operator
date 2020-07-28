package nodekeeper

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var StatusChangedPredicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		newNode, ok := e.MetaNew.(*corev1.Node)
		if !ok {
			return false
		}
		nodeLabels := newNode.GetLabels()
		for k, v := range nodeLabels {
			if k == "node-role.kubernetes.io/master" && v == "" {
				log.Info(fmt.Sprintf("Predicate denied reconciling master node: %s", newNode.Name))
				return false
			}
		}
		return true
	},
	GenericFunc: func(e event.GenericEvent) bool {
		newNodeName := e.Meta.GetName()
		nodeLabels := e.Meta.GetLabels()
		for k, v := range nodeLabels {
			if k == "node-role.kubernetes.io/master" && v == "" {
				log.Info(fmt.Sprintf("Predicate denied reconciling master node: %s", newNodeName))
				return false
			}
		}
		return true
	},
}

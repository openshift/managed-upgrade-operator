package machineconfigpool

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
)

func isWorkerPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			mp, ok := e.ObjectNew.(*machineconfigapi.MachineConfigPool)
			if !ok {
				return false
			}
			return isWorkerPool(mp.Name)
		},
		// Create is required to avoid reconciliation at controller initialisation.
		CreateFunc: func(e event.CreateEvent) bool {
			return isWorkerPool(e.Object.GetName())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isWorkerPool(e.Object.GetName())
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isWorkerPool(e.Object.GetName())
		},
	}
}

func isWorkerPool(name string) bool {
	return name == "worker"
}

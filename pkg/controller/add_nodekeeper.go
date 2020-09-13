package controller

import (
	"github.com/openshift/managed-upgrade-operator/pkg/controller/nodekeeper"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, nodekeeper.Add)
}

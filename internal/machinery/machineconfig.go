package machinery

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	machineconfigapi "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NodesUpgrading determines if worker nodes are currently upgrading by comparing
// MachineCount and UpdatedMachineCount
func (m *machinery) IsUpgrading(c client.Client, nodeType string, logger logr.Logger) (bool, error) {
	configPool := &machineconfigapi.MachineConfigPool{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: nodeType}, configPool)
	if err != nil {
		logger.Info("Failed to get %s node configpool object.", nodeType)
		return false, err
	}
	if configPool.Status.MachineCount != configPool.Status.UpdatedMachineCount {
		return true, nil
	}

	logger.Info(fmt.Sprintf("%s nodes are not upgrading", nodeType))
	return false, nil
}

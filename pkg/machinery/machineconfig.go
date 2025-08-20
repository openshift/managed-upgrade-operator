package machinery

import (
	"context"

	machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpgradingResult provides a struct to illustrate the upgrading result
type UpgradingResult struct {
	IsUpgrading  bool
	UpdatedCount int32
	MachineCount int32
}

// IsUpgrading determines if machines are currently upgrading by comparing
// MachineCount and UpdatedMachineCount
func (m *machinery) IsUpgrading(c client.Client, nodeType string) (*UpgradingResult, error) {
	configPool := &machineconfigv1.MachineConfigPool{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: nodeType}, configPool)
	if err != nil {
		return nil, err
	}

	return &UpgradingResult{
		IsUpgrading:  configPool.Status.MachineCount != configPool.Status.UpdatedMachineCount,
		UpdatedCount: configPool.Status.UpdatedMachineCount,
		MachineCount: configPool.Status.MachineCount,
	}, nil
}

package upgraders

import (
	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUpgraders(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Upgraders Suite")
}

// constructs and returns an upgradeConfig suitable for testing
func buildTestUpgraderConfig(controlPlaneTimeout int, scaleTimeOut int, nodeDrainTime int, upgradeWindowTimeout int, delayTimeout int) *upgraderConfig {
	return &upgraderConfig{
		Maintenance: maintenanceConfig{
			ControlPlaneTime: controlPlaneTimeout,
		},
		Scale: scaleConfig{
			TimeOut: scaleTimeOut,
		},
		NodeDrain: drain.NodeDrain{
			ExpectedNodeDrainTime: nodeDrainTime,
		},
		UpgradeWindow: upgradeWindow{
			TimeOut:      upgradeWindowTimeout,
			DelayTrigger: delayTimeout,
		},
	}
}

package nodekeeper

import (
	"fmt"

	"github.com/openshift/managed-upgrade-operator/pkg/drain"
)

type nodeKeeperConfig struct {
	NodeDrain drain.NodeDrain `yaml:"nodeDrain"`
}

func (nkc *nodeKeeperConfig) IsValid() error {
	if nkc.NodeDrain.Timeout < 0 {
		return fmt.Errorf("Config nodeDrain timeOut is invalid")
	}

	return nil
}

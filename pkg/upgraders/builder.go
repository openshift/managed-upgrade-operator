package upgraders

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/pkg/eventmanager"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

// ClusterUpgrader enables an implementation of a ClusterUpgrader
// Interface describing the functions of a cluster upgrader.
//go:generate mockgen -destination=mocks/cluster_upgrader.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgraders ClusterUpgrader
type ClusterUpgrader interface {
	UpgradeCluster(ctx context.Context, upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, error)
}

// ClusterUpgraderBuilder enables an implementation of a ClusterUpgraderBuilder
//go:generate mockgen -destination=mocks/cluster_upgrader_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/upgraders ClusterUpgraderBuilder
type ClusterUpgraderBuilder interface {
	NewClient(client.Client, configmanager.ConfigManager, metrics.Metrics, eventmanager.EventManager, upgradev1alpha1.UpgradeType) (ClusterUpgrader, error)
}

// NewBuilder returns a clusterUpgraderBuilder
func NewBuilder() ClusterUpgraderBuilder {
	return &clusterUpgraderBuilder{}
}

type clusterUpgraderBuilder struct{}

func (cub *clusterUpgraderBuilder) NewClient(c client.Client, cfm configmanager.ConfigManager, mc metrics.Metrics, nc eventmanager.EventManager, upgradeType upgradev1alpha1.UpgradeType) (ClusterUpgrader, error) {
	switch upgradeType {
	case upgradev1alpha1.OSD:
		cu, err := NewOSDUpgrader(c, cfm, mc, nc)
		if err != nil {
			return nil, err
		}
		return cu, nil
	case upgradev1alpha1.ARO:
		cu, err := NewAROUpgrader(c, cfm, mc, nc)
		if err != nil {
			return nil, err
		}
		return cu, nil
	default:
		cu, err := NewOSDUpgrader(c, cfm, mc, nc)
		if err != nil {
			return nil, err
		}
		return cu, nil
	}
}

package cluster_upgrader_builder

import (
	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/osd_cluster_upgrader"
)

// Interface describing the functions of a cluster upgrader.
//go:generate mockgen -destination=mocks/cluster_upgrader.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader_builder ClusterUpgrader
type ClusterUpgrader interface {
	UpgradeCluster(upgradeConfig *upgradev1alpha1.UpgradeConfig, logger logr.Logger) (upgradev1alpha1.UpgradePhase, *upgradev1alpha1.UpgradeCondition, error)
}

//go:generate mockgen -destination=mocks/cluster_upgrader_builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/cluster_upgrader_builder ClusterUpgraderBuilder
type ClusterUpgraderBuilder interface {
	NewClient(client.Client, metrics.Metrics, upgradev1alpha1.UpgradeType) (ClusterUpgrader, error)
}

func NewBuilder() ClusterUpgraderBuilder {
	return &clusterUpgraderBuilder{}
}

type clusterUpgraderBuilder struct{}

func (cub *clusterUpgraderBuilder) NewClient(c client.Client, mc metrics.Metrics, upgradeType upgradev1alpha1.UpgradeType) (ClusterUpgrader, error) {
	switch upgradeType {
	case upgradev1alpha1.OSD:
		cu, err := osd_cluster_upgrader.NewClient(c, mc)
		if err != nil {
			return nil, err
		}
		return cu, nil
	default:
		cu, err := osd_cluster_upgrader.NewClient(c, mc)
		if err != nil {
			return nil, err
		}
		return cu, nil

	}
}

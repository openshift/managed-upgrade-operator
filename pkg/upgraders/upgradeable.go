package upgraders

import (
	"context"
	"fmt"
	"time"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"

	"k8s.io/apimachinery/pkg/types"
)

const (
	upgradeCancelledArticleNumberForOSD = 6657541
	upgradeCancelledArticleNumberForROSA = 6541901
)

func (c *clusterUpgrader) isRosaCluster(ctx context.Context) (bool, error) {
	infraConfig := &configv1.Infrastructure{}
	if err := c.client.Get(ctx, types.NamespacedName{Name: "cluster"}, infraConfig); err != nil {
		return false, fmt.Errorf("failed fetching infrastructure config: %w", err)
	}

	platformStatus := infraConfig.Status.PlatformStatus

	if platformStatus != nil && platformStatus.AWS != nil {
		for _, resourceTag := range platformStatus.AWS.ResourceTags {
			if resourceTag.Key == "red-hat-clustertype" {
				return resourceTag.Value == "rosa", nil
			}
		}
	}

	return false, nil
}

func (c *clusterUpgrader) IsUpgradeable(ctx context.Context, logger logr.Logger) (bool, error) {
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.IsClusterUpgradable))
		return true, nil
	}

	clusterVersion, err := c.cvClient.GetClusterVersion()
	if err != nil {
		return false, err
	}
	currentVersion, err := cv.GetCurrentVersion(clusterVersion)
	if err != nil {
		return false, err
	}
	parsedCurrentVersion, err := semver.Parse(currentVersion)
	if err != nil {
		return false, err
	}

	desiredVersion := c.upgradeConfig.Spec.Desired.Version
	parsedDesiredVersion, err := semver.Parse(desiredVersion)
	if err != nil {
		return false, err
	}

	// if the upgradeable is false then we need to check the current version with upgrade version for y-stream update
	isRosaCluster, err := c.isRosaCluster(ctx)
	if err != nil {
		return false, err
	}

	var articleNumber int

	if isRosaCluster {
		articleNumber = upgradeCancelledArticleNumberForROSA
	} else {
		articleNumber = upgradeCancelledArticleNumberForOSD
	}

	upgradeTime, err := time.Parse(time.RFC3339, c.upgradeConfig.Spec.UpgradeAt)
	if err != nil {
		return false, fmt.Errorf("failed to parse spec.upgradeAt: %w", err)
	}
	upgradeTimeText := upgradeTime.UTC().Format(time.UnixDate)

	for _, condition := range clusterVersion.Status.Conditions {
		if condition.Type == configv1.OperatorUpgradeable && condition.Status == configv1.ConditionFalse && parsedDesiredVersion.Major >= parsedCurrentVersion.Major && parsedDesiredVersion.Minor > parsedCurrentVersion.Minor {
			return false, fmt.Errorf(
				"Cluster upgrade maintenance to version %s on %s has been cancelled due to unacknowledged user actions. See https://access.redhat.com/solutions/%d for more details.",
				desiredVersion,
				upgradeTimeText,
				articleNumber,
			)
		}
	}

	return true, nil
}

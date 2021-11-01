package upgraders

import (
	"context"
	"fmt"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
)

func (c *clusterUpgrader) IsUpgradeable(ctx context.Context, logger logr.Logger) (bool, error) {
	cV := &configv1.ClusterVersion{}
	uC := &upgradev1alpha1.UpgradeConfig{}

	//Check if the cluster is already upgrading -
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	// get current clusterversion
	cvVersion, err := cv.GetCurrentVersion(cV)
	if err != nil {
		logger.Error(err, "failed to get current cluster version")
		return false, err
	}

	parsedCvVersion, err := semver.Parse(cvVersion)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to parse current cluster version %s as semver", cvVersion))
		return false, err
	}

	// get the desired clusterversion from upgradeconfig
	ucVersion := uC.Spec.Desired.Version
	if err != nil {
		logger.Error(err, "failed to get current upgrade config desired version")
		return false, err
	}

	parsedUcVersion, err := semver.Parse(ucVersion)
	if err != nil {
		logger.Error(err, fmt.Sprintf("failed to parse upgrade config desired version %s as semver", ucVersion))
		return false, err
	}

	// check if the clusterversion has "Upgradeable" else return true
	for _, val := range cV.Status.Conditions {
		if val.Type != configv1.OperatorUpgradeable {
			return true, nil
		}
	}

	for _, condition := range cV.Status.Conditions {
		if condition.Type == configv1.OperatorUpgradeable && condition.Status == configv1.ConditionTrue {
			return true, nil
		} else if condition.Type == configv1.OperatorUpgradeable && condition.Status == configv1.ConditionFalse {
			// if the upgradeable is false then we need to check the current version with upgrade version for y-stream update
			if parsedUcVersion.Major >= parsedCvVersion.Major && parsedUcVersion.Minor > parsedCvVersion.Minor {
				return false, err
			} else if parsedUcVersion.Major >= parsedCvVersion.Major && parsedUcVersion.Minor == parsedCvVersion.Minor {
				return true, nil
			}
		}
	}
	return false, nil
}

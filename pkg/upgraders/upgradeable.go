package upgraders

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
)

func (c *clusterUpgrader) CheckUpgradeStatus(ctx context.Context, logger logr.Logger) (bool, error) {
	//Check if the cluster is already upgrading -
	upgradeCommenced, err := c.cvClient.HasUpgradeCommenced(c.upgradeConfig)
	if err != nil {
		return false, err
	}
	if upgradeCommenced {
		logger.Info(fmt.Sprintf("Skipping upgrade step %s", upgradev1alpha1.UpgradePreHealthCheck))
		return true, nil
	}

	//Pull the cluster-version - cvclient / get clusterversion
	//If Upgradable is not present at all then return "True" and proceed with the upgrade.
	//Upgradable = True/False
	// type: Upgradable && status: "True"/"False"

	/* If Upgradable = "True" then return true
	If Upgradable = "False" then
		1. check if the cluster upgrade is z-stream then return true and if it's y-stream then return false
	*/

	return false, nil
}

func (c *clusterUpgrader) IsUpgradable(ctx context.Context, uC *upgradev1alpha1.UpgradeConfig, cV *configv1.ClusterVersion, logger logr.Logger) (bool, error) {
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

	var vercompareidx int = 1

	cVversionAarray := strings.Split(cvVersion, ".") //cannot use parsedCvVersion - cannot use parsedCvVersion (variable of type semver.Version) as string value in argument to strings.Split
	uCversionAarray := strings.Split(ucVersion, ".")

	// check if the clusterversion has "Upgradeable" else return true
	// logic problem - fix the loop
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
			if cVversionAarray[vercompareidx] == uCversionAarray[vercompareidx] {
				return true, nil
			} else {
				return false, err
			}
		}
	}

	return false, nil
}

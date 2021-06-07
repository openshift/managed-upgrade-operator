// Package validation provides UpgradeConfig CR validation tools.
package validation

import (
	"fmt"
	"net/url"
	"runtime"
	"time"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-version-operator/pkg/cincinnati"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
)

const (
	defaultUpstreamServer = "https://api.openshift.com/api/upgrades_info/v1/graph"
)

// NewBuilder returns a validationBuilder object that implements the ValidationBuilder interface.
func NewBuilder() ValidationBuilder {
	return &validationBuilder{}
}

// Validator knows how to validate UpgradeConfig CRs.
//go:generate mockgen -destination=mocks/mockValidation.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/validation Validator
type Validator interface {
	IsValidUpgradeConfig(uC *upgradev1alpha1.UpgradeConfig, cV *configv1.ClusterVersion, logger logr.Logger) (ValidatorResult, error)
}

type validator struct{}

// ValidatorResult returns a type that enables validation of upgradeconfigs
type ValidatorResult struct {
	// Indicates that the UpgradeConfig is semantically and syntactically valid
	IsValid bool
	// Indicates that the UpgradeConfig should be actioned to conduct an upgrade
	IsAvailableUpdate bool
	// A message associated with the validation result
	Message string
}

// VersionComparison is an in used to compare versions
type VersionComparison int

const (
	// VersionUnknown is of type VersionComparision and is used to idicate an unknown version
	VersionUnknown VersionComparison = iota - 2
	// VersionDowngrade is of type VersionComparision and is used to idicate an version downgrade
	VersionDowngrade
	// VersionEqual is of type VersionComparision and is used to idicate version are equal
	VersionEqual
	// VersionUpgrade is of type VersionComparision and is used to idicate version is able to upgrade
	VersionUpgrade
)

func (v *validator) IsValidUpgradeConfig(uC *upgradev1alpha1.UpgradeConfig, cV *configv1.ClusterVersion, logger logr.Logger) (ValidatorResult, error) {
	// Validate upgradeAt as RFC3339
	_, err := time.Parse(time.RFC3339, uC.Spec.UpgradeAt)
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Failed to parse upgradeAt:%s during validation", uC.Spec.UpgradeAt),
		}, nil
	}

	// Validate desired version.
	dv := uC.Spec.Desired.Version
	version, err := cv.GetCurrentVersion(cV)
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           "Failed to get current cluster version during validation",
		}, err
	}

	// Check for valid SemVer and convert to SemVer.
	desiredVersion, err := semver.Parse(dv)
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Failed to parse desired version %s as semver", dv),
		}, nil
	}
	currentVersion, err := semver.Parse(version)
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Failed to parse desired version %s as semver", version),
		}, nil
	}

	// Compare versions to ascertain if upgrade should proceed.
	versionComparison, err := compareVersions(desiredVersion, currentVersion, logger)
	if err != nil {
		return ValidatorResult{
			IsValid:           true,
			IsAvailableUpdate: false,
			Message:           err.Error(),
		}, nil
	}

	switch versionComparison {
	case VersionUnknown:
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Desired version %s and current version %s could not be compared.", desiredVersion, currentVersion),
		}, nil
	case VersionDowngrade:
		return ValidatorResult{
			IsValid:           true,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Downgrades to desired version %s from %s are unsupported", desiredVersion, currentVersion),
		}, nil
	case VersionEqual:
		return ValidatorResult{
			IsValid:           true,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Desired version %s matches the current version %s", desiredVersion, currentVersion),
		}, nil
	case VersionUpgrade:
		logger.Info(fmt.Sprintf("Desired version %s validated as greater then current version %s", desiredVersion, currentVersion))
	}

	// Validate available version is in Cincinnati.
	desiredChannel := uC.Spec.Desired.Channel
	clusterId, err := uuid.Parse(string(cV.Spec.ClusterID))
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           "",
		}, nil
	}
	upstreamURI, err := url.Parse(getUpstreamURL(cV))
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           "",
		}, nil
	}

	updates, err := cincinnati.NewClient(clusterId, nil, nil).GetUpdates(upstreamURI, runtime.GOARCH, desiredChannel, currentVersion)
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           "",
		}, err
	}

	var cvoUpdates []configv1.Update
	for _, update := range updates {
		cvoUpdates = append(cvoUpdates, configv1.Update{
			Version: update.Version.String(),
			Image:   update.Image,
		})
	}

	// Check whether the desired version exists in availableUpdates
	found := false
	for _, v := range cvoUpdates {
		if v.Version == dv && !v.Force {
			found = true
		}
	}

	if !found {
		logger.Info(fmt.Sprintf("Failed to find the desired version %s in channel %s", desiredVersion, desiredChannel))
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("cannot find version %s in available updates", desiredVersion),
		}, nil
	}
	return ValidatorResult{
		IsValid:           true,
		IsAvailableUpdate: true,
		Message:           "UpgradeConfig is valid",
	}, nil
}

// compareVersions accepts desiredVersion and currentVersion strings as versions, converts
// them to semver and then compares them. Returns an indication of whether the desired
// version constitutes a downgrade, no-op or upgrade, or an error if no valid comparison can occur
func compareVersions(dV semver.Version, cV semver.Version, logger logr.Logger) (VersionComparison, error) {
	result := dV.Compare(cV)
	switch result {
	case -1:
		logger.Info(fmt.Sprintf("%s is less then %s", dV, cV))
		return VersionDowngrade, nil
	case 0:
		logger.Info(fmt.Sprintf("%s is equal to %s", dV, cV))
		return VersionEqual, nil
	case 1:
		logger.Info(fmt.Sprintf("%s is greater then %s", dV, cV))
		return VersionUpgrade, nil
	default:
		return VersionUnknown, fmt.Errorf("Semver comparison failed for unknown reason. Versions %s & %s", dV, cV)
	}

}

// getUpstreamURL retrieves the upstream URL from the ClusterVersion spec, defaulting to the default if not available
func getUpstreamURL(cV *configv1.ClusterVersion) string {
	upstream := string(cV.Spec.Upstream)
	if len(upstream) == 0 {
		upstream = defaultUpstreamServer
	}

	return upstream
}

// ValidationBuilder is a interface that enables ValidationBuiler implementations
//go:generate mockgen -destination=mocks/mockValidationBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/validation ValidationBuilder
type ValidationBuilder interface {
	NewClient() (Validator, error)
}

// validationBuilder is an empty struct that enables instantiation of this type and its
// implemented interface.
type validationBuilder struct{}

// NewClient returns a Validator interface or an error if one occurs.
func (vb *validationBuilder) NewClient() (Validator, error) {
	return &validator{}, nil
}

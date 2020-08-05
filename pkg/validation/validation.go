// Package validation provides UpgradeConfig CR validation tools.
package validation

import (
	"fmt"
	"net/url"
	"runtime"
	"time"

	"github.com/blang/semver"
	"github.com/google/uuid"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-version-operator/pkg/cincinnati"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"github.com/openshift/managed-upgrade-operator/pkg/osd_cluster_upgrader"

	"github.com/go-logr/logr"
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

type ValidatorResult struct {
	IsRollBack bool
	IsValid    bool
	Message    string
}

func (v *validator) IsValidUpgradeConfig(uC *upgradev1alpha1.UpgradeConfig, cV *configv1.ClusterVersion, logger logr.Logger) (ValidatorResult, error) {
	// Validate upgradeAt as RFC3339
	_, err := time.Parse(time.RFC3339, uC.Spec.UpgradeAt)
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    fmt.Sprintf("Failed to parse upgradeAt:%s during validation", uC.Spec.UpgradeAt),
		}, nil
	}

	// Validate desired version.
	dv := uC.Spec.Desired.Version
	cv, err := osd_cluster_upgrader.GetCurrentVersion(cV)
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    "Failed to get current cluster version during validation",
		}, err
	}

	// Check for valid SemVer and convert to SemVer.
	desiredVersion, err := semver.Parse(dv)
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    fmt.Sprintf("Failed to parse desired version %s as semver", dv),
		}, nil
	}
	currentVersion, err := semver.Parse(cv)
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    fmt.Sprintf("Failed to parse desired version %s as semver", cv),
		}, nil
	}

	// Compare versions to ascertain if upgrade should proceed.
	proceed := compareVersions(desiredVersion, currentVersion, logger)
	if !proceed {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: true,
			Message:    fmt.Sprintf("Desired version %s validated as greater then current version", desiredVersion),
		}, nil
	}
	logger.Info(fmt.Sprintf("Desired version %s validated as greater then current version %s", desiredVersion, currentVersion))

	// Validate available version is in Cincinnati.
	desiredChannel := uC.Spec.Desired.Channel
	clusterId, err := uuid.Parse(string(cV.Spec.ClusterID))
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    "",
		}, nil
	}
	upstreamURI, err := url.Parse(string(cV.Spec.Upstream))
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    "",
		}, nil
	}

	updates, err := cincinnati.NewClient(clusterId, nil, nil).GetUpdates(upstreamURI, runtime.GOARCH, desiredChannel, currentVersion)
	if err != nil {
		return ValidatorResult{
			IsValid:    false,
			IsRollBack: false,
			Message:    "",
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
			IsValid:    false,
			IsRollBack: false,
			Message:    fmt.Sprintf("cannot find version %s in available updates", desiredVersion),
		}, nil
	}
	return ValidatorResult{
		IsValid:    true,
		IsRollBack: false,
		Message:    "UpgradeConfig is valid",
	}, nil
}

// compareVersions accepts desiredVersion and currentVersion strings as versions, converts
// them to semver and then compares them. Returning true only if desiredVersion > currentVersion.
func compareVersions(dV semver.Version, cV semver.Version, logger logr.Logger) bool {
	result := dV.Compare(cV)
	switch result {
	case -1:
		logger.Info(fmt.Sprintf("%s is less then %s", dV, cV))
		logger.Info(fmt.Sprintf("Downgrading cluster is not supported. Not Proceeding to %s", dV))
		return false
	case 0:
		logger.Info(fmt.Sprintf("%s is equal to %s", dV, cV))
		logger.Info(fmt.Sprintf("Cluster is already on version %s", cV))
		return false
	case 1:
		logger.Info(fmt.Sprintf("%s is greater then %s", dV, cV))
		return true
	default:
		logger.Info(fmt.Sprintf("Semver comparison failed for unknown reason. Versions %s & %s", dV, cV))
		return false
	}

}

//go:generate mockgen -destination=mocks/mockValidationBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/validation ValidationBuilder
type ValidationBuilder interface {
	NewClient() (Validator, error)
}

// validationBuilder is an empty struct that enables instantiation of this type and its
// implimented interface.
type validationBuilder struct{}

// NewClient returns a Validator interface or an error if one occurs.
func (vb *validationBuilder) NewClient() (Validator, error) {
	return &validator{}, nil
}

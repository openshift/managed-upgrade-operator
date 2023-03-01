// Package validation provides UpgradeConfig CR validation tools.
package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/openshift/managed-upgrade-operator/pkg/configmanager"

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-version-operator/pkg/cincinnati"
	"github.com/openshift/library-go/pkg/image/dockerv1client"
	imagereference "github.com/openshift/library-go/pkg/image/reference"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/api/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultUpstreamServer = "https://api.openshift.com/api/upgrades_info/v1/graph"
)

// NewBuilder returns a validationBuilder object that implements the ValidationBuilder interface.
func NewBuilder() ValidationBuilder {
	return &validationBuilder{}
}

// Validator knows how to validate UpgradeConfig CRs.
//
//go:generate mockgen -destination=mocks/mockValidation.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/validation Validator
type Validator interface {
	IsValidUpgradeConfig(c client.Client, uC *upgradev1alpha1.UpgradeConfig, cV *configv1.ClusterVersion, logger logr.Logger) (ValidatorResult, error)
}

type validator struct {
	// Indicates that Cincinnati version validation should be performed
	Cincinnati bool
}

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

// IsValidUpgradeConfig checks the validity of UpgradeConfig CRs
func (v *validator) IsValidUpgradeConfig(c client.Client, uC *upgradev1alpha1.UpgradeConfig, cV *configv1.ClusterVersion, logger logr.Logger) (ValidatorResult, error) {
	validationPassed := ValidatorResult{
		IsValid:           true,
		IsAvailableUpdate: true,
		Message:           "Upgrade config is valid",
	}

	// Validate upgradeAt as RFC3339
	upgradeAt := uC.Spec.UpgradeAt
	_, err := time.Parse(time.RFC3339, upgradeAt)
	if err != nil {
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           fmt.Sprintf("Failed to parse upgradeAt:%s during validation", upgradeAt),
		}, err
	}

	ucImage := uC.Spec.Desired.Image
	ucVersion := uC.Spec.Desired.Version
	ucChannel := uC.Spec.Desired.Channel

	// Validate the spec.desired.image if it is specified
	// Write the spec.desired.version from the image version since we need the version in the history
	if ucImage != "" {
		digestVersion, err := fetchImageVersion(ucImage)
		if err != nil {
			return ValidatorResult{
				IsValid:           false,
				IsAvailableUpdate: false,
				Message:           err.Error(),
			}, err
		}
		if digestVersion != ucVersion {
			err = updateImageVersion(c, digestVersion, uC)
			if err != nil {
				return ValidatorResult{
					IsValid:           false,
					IsAvailableUpdate: false,
					Message:           err.Error(),
				}, err
			}
		}
		err = imageValidation(ucImage)
		if err != nil {
			return ValidatorResult{
				IsValid:           false,
				IsAvailableUpdate: false,
				Message:           err.Error(),
			}, err
		}
		return validationPassed, nil
	}

	// If there's no version and channel either, this is invalid
	if ucVersion == "" && ucChannel == "" {
		// Return invalid by default
		return ValidatorResult{
			IsValid:           false,
			IsAvailableUpdate: false,
			Message:           "Not able to validate the upgrade config, either image or (channel + version) needs to be provided",
		}, nil
	}

	// For all versions, first verify it's an actual upgrade and not a same-version or downgrade
	versionValid, versionAvailable, err := versionValidation(ucVersion, cV, logger)
	if err != nil {
		return ValidatorResult{
			IsValid:           versionValid,
			IsAvailableUpdate: versionAvailable,
			Message:           err.Error(),
		}, err
	}

	// For y-stream upgrades only, verify the upgrade edge in Cincinnati
	if v.Cincinnati && ucChannel != cV.Spec.Channel {
		cvoUpdates, err := fetchCVOUpdates(cV, uC)
		if err != nil {
			return ValidatorResult{
				IsValid:           false,
				IsAvailableUpdate: false,
				Message:           err.Error(),
			}, err
		}
		err = channelValidation(uC, cvoUpdates, logger)
		if err != nil {
			return ValidatorResult{
				IsValid:           false,
				IsAvailableUpdate: false,
				Message:           err.Error(),
			}, err
		}
	}

	// For z-stream upgrades only, verify that CVO knows about the version already
	if ucChannel == cV.Spec.Channel {
		updateAvailable := false
		// Check if the version is in the AvailableUpdates list
		for _, update := range cV.Status.AvailableUpdates {
			if update.Version == uC.Spec.Desired.Version {
				updateAvailable = true
			}
		}
		// Check if the version is in the ConditionalUpdates list if the version wasn't found in AvailableUpdates
		if !updateAvailable {
			for _, update := range cV.Status.ConditionalUpdates {
				if update.Release.Version == uC.Spec.Desired.Version {
					updateAvailable = true
				}
			}
		}
		// If the version isn't in either list, then it's not a valid upgrade
		if !updateAvailable {
			return ValidatorResult{
				IsValid:           false,
				IsAvailableUpdate: false,
				Message:           fmt.Sprintf("version %s not found in clusterversion available or conditional updates", uC.Spec.Desired.Version),
			}, err
		}
	}

	return validationPassed, nil
}

// compareVersions accepts desiredVersion and currentVersion strings as versions, converts
// them to semver and then compares them. Returns an indication of whether the desired
// version constitutes a downgrade, no-op or upgrade, or an error if no valid comparison can occur
func compareVersions(dV semver.Version, cV semver.Version, logger logr.Logger) (VersionComparison, error) {
	result := dV.Compare(cV)
	switch result {
	case -1:
		logger.Info(fmt.Sprintf("%s is less than %s", dV, cV))
		return VersionDowngrade, nil
	case 0:
		logger.Info(fmt.Sprintf("%s is equal to %s", dV, cV))
		return VersionEqual, nil
	case 1:
		logger.Info(fmt.Sprintf("%s is greater than %s", dV, cV))
		return VersionUpgrade, nil
	default:
		return VersionUnknown, fmt.Errorf("semver comparison failed for unknown reason. Versions %s & %s", dV, cV)
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
//
//go:generate mockgen -destination=mocks/mockValidationBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/validation ValidationBuilder
type ValidationBuilder interface {
	NewClient(configmanager.ConfigManager) (Validator, error)
}

// validationBuilder is an empty struct that enables instantiation of this type and its
// implemented interface.
type validationBuilder struct{}

// NewClient returns a Validator interface or an error if one occurs.
func (vb *validationBuilder) NewClient(cfm configmanager.ConfigManager) (Validator, error) {
	cfg := &ValidationConfig{}
	err := cfm.Into(cfg)
	if err != nil {
		return nil, err
	}

	return &validator{
		Cincinnati: cfg.Validation.Cincinnati,
	}, nil
}

// fetchImageVersion function returns the image version from the image digest
func fetchImageVersion(image string) (string, error) {
	ref, _ := imagereference.Parse(image)
	manifesturl := url.URL{
		Scheme: "https",
		Host:   ref.Registry,
		Path:   "v2" + "/" + ref.Namespace + "/" + ref.Name + "/" + "manifests" + "/" + ref.ID,
	}

	body, err := runHTTP(manifesturl.String())
	if len(body) == 0 {
		return "", fmt.Errorf("failed to fetch image manifest digest: %s needs to be a valid release image", image)
	}
	if err != nil {
		return "", err
	}

	manifest := &dockerv1client.DockerImageManifest{}
	if err := parse(body, &manifest); err != nil {
		return "", err
	}
	bloburl := url.URL{
		Scheme: "https",
		Host:   ref.Registry,
		Path:   "v2" + "/" + ref.Namespace + "/" + ref.Name + "/" + "blobs" + "/" + manifest.Config.Digest,
	}

	resbody, err := runHTTP(bloburl.String())
	if len(resbody) == 0 {
		return "", fmt.Errorf("failed to fetch blobs for image manifest digest: %s needs to be a valid release image", image)
	}
	if err != nil {
		return "", err
	}

	imageConfig := &dockerv1client.DockerImageConfig{}
	if err := parse(resbody, &imageConfig); err != nil {
		return "", err
	}
	return imageConfig.Config.Labels["io.openshift.release"], nil
}

func runHTTP(url string) ([]byte, error) {

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	client := http.Client{
		Timeout: time.Second * 20,
	}

	res, getErr := client.Do(req)
	if getErr != nil {
		return nil, getErr
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("return code is not 200")
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, readErr
	}
	return body, nil
}

func parse(body []byte, v interface{}) error {
	jsonErr := json.Unmarshal(body, v)
	if jsonErr != nil {
		return jsonErr
	}
	return nil
}

func updateImageVersion(c client.Client, v string, upgradeConfig *upgradev1alpha1.UpgradeConfig) error {
	// Update the version in UpgradeConfigSpec
	upgradeConfig.Spec.Desired.Version = v
	err := c.Update(context.TODO(), upgradeConfig)
	if err != nil {
		return err
	}

	// Update the version in UpgradeConfig.Status.History
	history := upgradeConfig.Status.History.GetHistory(upgradeConfig.Spec.Desired.Version)
	if history == nil {
		for _, h := range upgradeConfig.Status.History {
			if h.Phase == upgradev1alpha1.UpgradePhaseNew {
				h.Version = upgradeConfig.Spec.Desired.Version
				upgradeConfig.Status.History[0] = h
				err := c.Status().Update(context.TODO(), upgradeConfig)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Validate the given spec.desired.image
func imageValidation(image string) error {
	ref, err := imagereference.Parse(image)
	if err != nil {
		return fmt.Errorf("failed to parse image %s: must be a valid image pull spec:%v", image, err)
	}

	if ref.Registry == "" {
		return fmt.Errorf("failed to parse image:%s must be a valid image pull spec: no registry specified", image)
	}

	if ref.Namespace == "" {
		return fmt.Errorf("failed to parse image:%s must be a valid image pull spec: no repository specified", image)
	}

	if ref.Name == "" {
		return fmt.Errorf("failed to parse image:%s must be a valid image pull spec: no image name specified", image)
	}

	if ref.ID == "" {
		return fmt.Errorf("failed to parse image:%s must be a valid image pull spec: no image digest specified", image)
	}

	return nil
}

// Validate the given spec.desired.version
func versionValidation(ucVersion string, cV *configv1.ClusterVersion, logger logr.Logger) (valid bool, available bool, err error) {
	// Check for valid SemVer and convert to SemVer.
	parsedUcVersion, err := semver.Parse(ucVersion)
	if err != nil {
		logger.Error(err, fmt.Sprintf("failed to parse upgrade config desired version %s as semver", ucVersion))
		return false, false, fmt.Errorf("failed to parse upgrade config desired version %s as semver: %w", ucVersion, err)
	}

	cvVersion, err := cv.GetCurrentVersion(cV)
	if err != nil {
		logger.Error(err, "failed to get current cluster version during validation")
		return false, false, fmt.Errorf("failed to get current cluster version during validation: %w", err)
	}
	parsedCvVersion, err := semver.Parse(cvVersion)
	if err != nil {
		logger.Error(err, fmt.Sprintf("failed to parse current cluster version %s as semver", cvVersion))
		return false, false, fmt.Errorf("failed to parse current cluster version %s as semver: %w", cvVersion, err)
	}

	// Compare versions to ascertain if upgrade should proceed.
	versionComparison, err := compareVersions(parsedUcVersion, parsedCvVersion, logger)
	if err != nil {
		return false, false, fmt.Errorf("failed to compare versions: %w", err)
	}
	switch versionComparison {
	case VersionUnknown:
		return false, false, fmt.Errorf("desired version %s and current version %s could not be compared", ucVersion, cvVersion)
	case VersionDowngrade:
		return true, false, fmt.Errorf("downgrades to desired version %s from %s are unsupported", ucVersion, cvVersion)
	case VersionEqual:
		return true, false, fmt.Errorf("desired version %s matches the current version %s", ucVersion, cvVersion)
	case VersionUpgrade:
		logger.Info(fmt.Sprintf("Desired version %s validated as greater than current version %s", ucVersion, cvVersion))
	}

	return true, true, nil
}

// Validate the given spec.desired.channel
func channelValidation(uC *upgradev1alpha1.UpgradeConfig, cvoUpdates []configv1.Update, logger logr.Logger) error {
	ucDesired := uC.Spec.Desired

	// Check whether the desired version exists in availableUpdates
	found := false
	for _, v := range cvoUpdates {
		if v.Version == ucDesired.Version && !v.Force {
			found = true
		}
	}

	if !found {
		logger.Info(fmt.Sprintf("Failed to find the desired version %s in channel %s", ucDesired.Version, ucDesired.Channel))
		return fmt.Errorf("cannot find version %s in available updates", ucDesired.Version)
	}

	return nil
}

// Fetch the available upgrade from upstream with the given version
func fetchCVOUpdates(cV *configv1.ClusterVersion, uc *upgradev1alpha1.UpgradeConfig) ([]configv1.Update, error) {
	clusterId, err := uuid.Parse(string(cV.Spec.ClusterID))
	if err != nil {
		return nil, err
	}
	upstreamURI, err := url.Parse(getUpstreamURL(cV))
	if err != nil {
		return nil, err
	}

	cvVersion, _ := cv.GetCurrentVersion(cV)
	parsedCvVersion, _ := semver.Parse(cvVersion)

	transport := &http.Transport{}
	ctx := context.TODO()

	// Fetch available updates by version in Cincinnati.
	_, updates, _, err := cincinnati.NewClient(clusterId, transport).GetUpdates(ctx, upstreamURI, runtime.GOARCH, uc.Spec.Desired.Channel, parsedCvVersion)
	if err != nil {
		return nil, err
	}

	if len(updates) > 0 {
		var cvoUpdates []configv1.Update
		for _, update := range updates {
			cvoUpdates = append(cvoUpdates, configv1.Update{
				Version: update.Version,
				Image:   update.Image,
			})
		}
		return cvoUpdates, err
	}

	return nil, fmt.Errorf("no available upgrade for the given clusterversion %s", cvVersion)
}

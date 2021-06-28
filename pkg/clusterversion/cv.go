package clusterversion

import (
	"context"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
)

var (
	// OSD_CV_NAME is the name of cluster version singleton
	OSD_CV_NAME = "version"
)

// ClusterVersion interface enables implementations of the ClusterVersion
//go:generate mockgen -destination=mocks/mockClusterVersion.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/clusterversion ClusterVersion
type ClusterVersion interface {
	GetClusterVersion() (*configv1.ClusterVersion, error)
	HasUpgradeCommenced(*upgradev1alpha1.UpgradeConfig) (bool, error)
	EnsureDesiredVersion(uc *upgradev1alpha1.UpgradeConfig) (bool, error)
	HasUpgradeCompleted(*configv1.ClusterVersion, *upgradev1alpha1.UpgradeConfig) bool
	HasDegradedOperators() (*HasDegradedOperatorsResult, error)
}

// ClusterVersionBuilder returns a ClusterVersion interface
//go:generate mockgen -destination=mocks/mockClusterVersionBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/clusterversion ClusterVersionBuilder
type ClusterVersionBuilder interface {
	New(client.Client) ClusterVersion
}

type clusterVersionClient struct {
	client client.Client
}

type clusterVersionClientBuilder struct{}

// NewCVClient returns a ClusterVersion interface
func NewCVClient(c client.Client) ClusterVersion {
	return &clusterVersionClient{c}
}

// NewBuilder returns a CluserVersionBuilder type
func NewBuilder() ClusterVersionBuilder {
	return &clusterVersionClientBuilder{}
}

func (cvb *clusterVersionClientBuilder) New(c client.Client) ClusterVersion {
	return NewCVClient(c)
}

// GetClusterVersion gets the ClusterVersion CR
func (c *clusterVersionClient) GetClusterVersion() (*configv1.ClusterVersion, error) {
	cv := &configv1.ClusterVersion{}
	err := c.client.Get(context.TODO(), types.NamespacedName{Name: OSD_CV_NAME}, cv)
	if err != nil {
		return nil, err
	}

	return cv, err
}

func (c *clusterVersionClient) EnsureDesiredVersion(uc *upgradev1alpha1.UpgradeConfig) (bool, error) {
	clusterVersion, err := c.GetClusterVersion()
	if err != nil {
		return false, err
	}

	// Move the cluster to the same channel first
	desired := uc.Spec.Desired
	if clusterVersion.Spec.Channel != desired.Channel {
		clusterVersion.Spec.Channel = desired.Channel
		err = c.client.Update(context.TODO(), clusterVersion)
		if err != nil {
			return false, err
		}

		// Retrieve the updated version
		clusterVersion, err = c.GetClusterVersion()
		if err != nil {
			return false, err
		}
	}

	// The CVO may need time sync the version before launching the upgrade
	updateAvailable := false
	for _, update := range clusterVersion.Status.AvailableUpdates {
		if update.Version == desired.Version && update.Image != "" {
			updateAvailable = true
		}
	}
	if !updateAvailable {
		return false, nil
	}

	clusterVersion.Spec.Overrides = []configv1.ComponentOverride{}
	clusterVersion.Spec.DesiredUpdate = &configv1.Update{Version: uc.Spec.Desired.Version}
	err = c.client.Update(context.TODO(), clusterVersion)
	if err != nil {
		return false, err
	}

	return true, nil
}

// HasDegradedOperatorsResult holds fields that describe a degraded operator
type HasDegradedOperatorsResult struct {
	Degraded []string
}

func (c *clusterVersionClient) HasDegradedOperators() (*HasDegradedOperatorsResult, error) {
	operatorList := &configv1.ClusterOperatorList{}
	err := c.client.List(context.TODO(), operatorList, []client.ListOption{}...)
	if err != nil {
		return &HasDegradedOperatorsResult{
			Degraded: []string{},
		}, err
	}

	degradedOperators := []string{}
	for _, co := range operatorList.Items {
		for _, condition := range co.Status.Conditions {
			if (condition.Type == configv1.OperatorDegraded && condition.Status == configv1.ConditionTrue) || (condition.Type == configv1.OperatorAvailable && condition.Status == configv1.ConditionFalse) {
				degradedOperators = append(degradedOperators, co.Name)
				break
			}
		}
	}

	return &HasDegradedOperatorsResult{
		Degraded: degradedOperators,
	}, err
}

func (c *clusterVersionClient) HasUpgradeCompleted(cv *configv1.ClusterVersion, uc *upgradev1alpha1.UpgradeConfig) bool {
	isCompleted := false
	for _, c := range cv.Status.History {
		if c.Version == uc.Spec.Desired.Version {
			if c.State == configv1.CompletedUpdate {
				isCompleted = true
			}
		}
	}

	return isCompleted
}

// isEqualVersion compare the upgrade version state for cv and uc
func isEqualVersion(cv *configv1.ClusterVersion, uc *upgradev1alpha1.UpgradeConfig) bool {
	if cv.Spec.DesiredUpdate != nil &&
		cv.Spec.DesiredUpdate.Version == uc.Spec.Desired.Version {
		return true
	}

	return false
}

// hasUpgradeCommenced checks if the upgrade has commenced
func (c *clusterVersionClient) HasUpgradeCommenced(uc *upgradev1alpha1.UpgradeConfig) (bool, error) {

	clusterVersion, err := c.GetClusterVersion()
	if err != nil {
		return false, err
	}

	if !isEqualVersion(clusterVersion, uc) {
		return false, nil
	}

	return true, nil
}

// GetHistory returns a configv1.UpdateHistory from a ClusterVersion
func GetHistory(clusterVersion *configv1.ClusterVersion, version string) *configv1.UpdateHistory {
	for _, history := range clusterVersion.Status.History {
		if history.Version == version {
			return &history
		}
	}

	return nil
}

// GetCurrentVersion strings a version as a string and error
func GetCurrentVersion(clusterVersion *configv1.ClusterVersion) (string, error) {
	var gotVersion string
	var latestCompletionTime *metav1.Time = nil
	for _, history := range clusterVersion.Status.History {
		if history.State == configv1.CompletedUpdate {
			if latestCompletionTime == nil || history.CompletionTime.After(latestCompletionTime.Time) {
				gotVersion = history.Version
				latestCompletionTime = history.CompletionTime
			}
		}
	}

	if len(gotVersion) == 0 {
		return gotVersion, fmt.Errorf("Failed to get current version")
	}

	return gotVersion, nil
}

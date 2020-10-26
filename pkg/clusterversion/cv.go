package clusterversion

import (
	"context"

	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/mockClusterVersion.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/clusterversion ClusterVersion
type ClusterVersion interface {
	GetClusterVersion() (*configv1.ClusterVersion, error)
	HasUpgradeCommenced(*upgradev1alpha1.UpgradeConfig) (bool, error)
}

//go:generate mockgen -destination=mocks/mockClusterVersionBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/clusterversion ClusterVersionBuilder
type ClusterVersionBuilder interface {
	New(client.Client) ClusterVersion
}

type clusterVersionClient struct {
	client client.Client
}

type clusterVersionClientBuilder struct{}

func NewCVClient(c client.Client) ClusterVersion {
	return &clusterVersionClient{c}
}

func NewBuilder() ClusterVersionBuilder {
	return &clusterVersionClientBuilder{}
}

func (cvb *clusterVersionClientBuilder) New(c client.Client) ClusterVersion {
	return NewCVClient(c)
}

// GetClusterVersion gets the ClusterVersion CR
func (c *clusterVersionClient) GetClusterVersion() (*configv1.ClusterVersion, error) {
	cvList := &configv1.ClusterVersionList{}
	err := c.client.List(context.TODO(), cvList)
	if err != nil {
		return nil, err
	}

	// ClusterVersion is a singleton
	for _, cv := range cvList.Items {
		return &cv, nil
	}

	return nil, errors.NewNotFound(schema.GroupResource{Group: configv1.GroupName, Resource: "ClusterVersion"}, "ClusterVersion")
}

// isEqualVersion compare the upgrade version state for cv and uc
func isEqualVersion(cv *configv1.ClusterVersion, uc *upgradev1alpha1.UpgradeConfig) bool {
	if cv.Spec.DesiredUpdate != nil &&
		cv.Spec.DesiredUpdate.Version == uc.Spec.Desired.Version &&
		cv.Spec.Channel == uc.Spec.Desired.Channel {
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

func GetHistory(clusterVersion *configv1.ClusterVersion, version string) *configv1.UpdateHistory {
	var result *configv1.UpdateHistory
	for _, history := range clusterVersion.Status.History {
		if history.Version == version {
			result = &history
		}
	}

	return result
}

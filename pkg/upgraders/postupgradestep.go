package upgraders

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/openshift/managed-upgrade-operator/config"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fioNamespace    string = "openshift-file-integrity"
	fioObject       string = "osd-fileintegrity"
	frOCMBaseDomain string = "openshiftusgov.com"
)

var reinitAnnotation = map[string]string{"file-integrity.openshift.io/re-init": ""}

// ugConfigManager stores the configManager section of the upgradeconfig manager configmap
type ugConfigManager struct {
	Source     string `yaml:"source"`
	OcmBaseURL string `yaml:"ocmBaseUrl"`
}

// ugConfigManagerSpec stores the config.yaml section of the upgradeconfig manager configmap
type ugConfigManagerSpec struct {
	Config ugConfigManager `yaml:"configManager"`
}

// PostUpgradeProcedures are any misc tasks that are needed to be completed after an upgrade has finished to ensure healthy state
// Currently the only task is to reinit file integrity operator due to changes that come from upgrades
func (c *clusterUpgrader) PostUpgradeProcedures(ctx context.Context, logger logr.Logger) (bool, error) {

	frCluster, err := c.frClusterCheck(ctx)
	if err != nil {
		return false, err
	}
	if !frCluster {
		logger.Info("Non-FedRAMP environment...skipping PostUpgradeFIOReInit ")
		return true, nil
	}
	err = c.postUpgradeFIOReInit(ctx, logger)
	if err != nil {
		return false, err
	}
	return true, nil
}

// frClusterCheck checks to see if the upgrading cluster is a FedRAMP cluster to determine if we need to re-init the File Integrity Operator
func (c *clusterUpgrader) frClusterCheck(ctx context.Context) (bool, error) {
	ocmConfig := &corev1.ConfigMap{}
	err := c.client.Get(context.TODO(), client.ObjectKey{Namespace: config.OperatorNamespace, Name: config.ConfigMapName}, ocmConfig)

	if err != nil {
		return false, fmt.Errorf("failed to fetch %s config map to parse: %v", config.ConfigMapName, err)
	}

	var cm ugConfigManagerSpec
	data := fmt.Sprint(ocmConfig.Data["config.yaml"])
	err = yaml.Unmarshal([]byte(data), &cm)

	if err != nil {
		return false, fmt.Errorf("failed to parse %s config map for OCM URL: %v", config.ConfigMapName, err)
	}

	if cm.Config.Source == "OCM" {
		ocmBaseUrl, err := url.Parse(cm.Config.OcmBaseURL)
		if err != nil {
			return false, fmt.Errorf("failed to parse %s config map for OCM URL: %v", config.ConfigMapName, err)
		}
		if !strings.Contains(ocmBaseUrl.Host, frOCMBaseDomain) {
			return false, nil
		}
	} else {
		return false, nil
	}
	return true, nil
}

// postUpgradeFIOReInit reinitializes the AIDE DB in file integrity operator to track file changes due to upgrades
func (c *clusterUpgrader) postUpgradeFIOReInit(ctx context.Context, logger logr.Logger) error {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "fileintegrity.openshift.io",
		Kind:    "FileIntegrity",
		Version: "v1alpha1",
	})

	logger.Info("FedRAMP Environment...Fetching File Integrity for re-initialization")
	err := c.client.Get(context.TODO(), client.ObjectKey{Namespace: fioNamespace, Name: fioObject}, u)
	if err != nil {
		return fmt.Errorf("failed to fetch file integrity %s in %s namespace: %v", fioObject, fioNamespace, err)
	}

	logger.Info("Setting re-init annotation")
	u.SetAnnotations(reinitAnnotation)
	err = c.client.Update(context.TODO(), u)
	if err != nil {
		logger.Error(err, "Failed to annotate File Integrity object")
		return err
	}
	logger.Info("File Integrity Operator AIDE Datbase reinitialized")
	return nil
}

package upgraders

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	fioNamespace string = "openshift-file-integrity"
	fioObject    string = "osd-fileintegrity"
)

var reinitAnnotation = map[string]string{"file-integrity.openshift.io/re-init": ""}

// PostUpgradeProcedures are any misc tasks that are needed to be completed after an upgrade has finished to ensure healthy state
// Currently the only task is to reinit file integrity operator due to changes that come from upgrades
func (c *clusterUpgrader) PostUpgradeProcedures(ctx context.Context, logger logr.Logger) (bool, error) {

	if !c.config.Environment.IsFedramp() {
		logger.Info("Non-FIO environment...skipping PostUpgradeFIOReInit ")
		return true, nil
	}
	err := c.postUpgradeFIOReInit(ctx, logger)
	if err != nil {
		return false, err
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
	logger.Info("File Integrity Operator AIDE Database reinitialized")
	return nil
}

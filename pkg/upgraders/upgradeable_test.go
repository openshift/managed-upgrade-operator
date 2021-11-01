package upgraders

import (
	"context"
	"testing"

	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	upgradev1alpha1 "github.com/openshift/managed-upgrade-operator/pkg/apis/upgrade/v1alpha1"
	cv "github.com/openshift/managed-upgrade-operator/pkg/clusterversion"
)

func Test_clusterUpgrader_IsUpgradable(t *testing.T) {
	type fields struct {
		steps                []upgradesteps.UpgradeStep
		client               client.Client
		metrics              metrics.Metrics
		cvClient             cv.ClusterVersion
		notifier             eventmanager.EventManager
		scaler               scaler.Scaler
		availabilityCheckers ac.AvailabilityCheckers
		upgradeConfig        *upgradev1alpha1.UpgradeConfig
		drainstrategyBuilder drain.NodeDrainStrategyBuilder
		maintenance          maintenance.Maintenance
		machinery            machinery.Machinery
		config               *upgraderConfig
	}
	type args struct {
		ctx    context.Context
		uC     *upgradev1alpha1.UpgradeConfig
		cV     *configv1.ClusterVersion
		logger logr.Logger
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterUpgrader{
				steps:                tt.fields.steps,
				client:               tt.fields.client,
				metrics:              tt.fields.metrics,
				cvClient:             tt.fields.cvClient,
				notifier:             tt.fields.notifier,
				scaler:               tt.fields.scaler,
				availabilityCheckers: tt.fields.availabilityCheckers,
				upgradeConfig:        tt.fields.upgradeConfig,
				drainstrategyBuilder: tt.fields.drainstrategyBuilder,
				maintenance:          tt.fields.maintenance,
				machinery:            tt.fields.machinery,
				config:               tt.fields.config,
			}
			got, err := c.IsUpgradable(tt.args.ctx, tt.args.uC, tt.args.cV, tt.args.logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("clusterUpgrader.IsUpgradable() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("clusterUpgrader.IsUpgradable() = %v, want %v", got, tt.want)
			}
		})
	}
}

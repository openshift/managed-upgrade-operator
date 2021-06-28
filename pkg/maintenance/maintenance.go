package maintenance

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Maintenance enables implementation of a maintenance interface type
//go:generate mockgen -destination=mocks/maintenance.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/maintenance Maintenance
type Maintenance interface {
	StartControlPlane(endsAt time.Time, version string, ignoredAlerts []string) error
	SetWorker(endsAt time.Time, version string, count int32) error
	EndControlPlane() error
	EndWorker() error
	EndSilences(comment string) error
	IsActive() (bool, error)
}

// MaintenanceBuilder enables an implementation of a maintenancebuilder interface type
//go:generate mockgen -destination=mocks/maintenanceBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/maintenance MaintenanceBuilder
type MaintenanceBuilder interface {
	NewClient(client client.Client) (Maintenance, error)
}

// NewBuilder returns a MaintenanceBuilder
func NewBuilder() MaintenanceBuilder {
	return &alertManagerMaintenanceBuilder{}
}

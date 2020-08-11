package maintenance

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/maintenance.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/maintenance Maintenance
type Maintenance interface {
	StartControlPlane(endsAt time.Time, version string, ignoredAlerts []string) error
	SetWorker(endsAt time.Time, version string) error
	EndControlPlane() error
	EndWorkers() error
	EndSilences(comment string) error
	IsActive() (bool, error)
}

//go:generate mockgen -destination=mocks/maintenanceBuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/maintenance MaintenanceBuilder
type MaintenanceBuilder interface {
	NewClient(client client.Client) (Maintenance, error)
}

func NewBuilder() MaintenanceBuilder {
	return &alertManagerMaintenanceBuilder{}
}

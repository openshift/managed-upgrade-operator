package maintenance

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

//go:generate mockgen -destination=../../util/mocks/$GOPACKAGE/maintenance.go -package=$GOPACKAGE github.com/openshift/managed-upgrade-operator/pkg/maintenance Maintenance
type Maintenance interface {
	StartControlPlane(endsAt time.Time) error
	StartWorker(endsAt time.Time) error
	End() error
}

func NewClient(client client.Client) (Maintenance, error) {
	amm, err := newAlertManagerMaintenance(client)
	if err != nil {
		return nil, err
	}

	return amm, nil
}

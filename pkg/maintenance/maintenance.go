package maintenance

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type Maintenance interface {
	Start(endsAt time.Time) error
	End() error
}

func NewClient(client client.Client) (Maintenance, error) {
	amm, err := newAlertManagerMaintenance(client)
	if err != nil {
		return nil, err
	}

	return amm, nil
}

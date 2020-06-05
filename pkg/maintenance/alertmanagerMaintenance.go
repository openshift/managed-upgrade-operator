package maintenance

import (
	"context"
	"github.com/coreos/pkg/multierror"
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/managed-upgrade-operator/config"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

var (
	alertManagerNamespace          = "openshift-monitoring"
	alertManagerRouteName          = "alertmanager-main"
	alertManagerServiceAccountName = "prometheus-k8s"
	alertManagerBasePath           = "/api/v2/"
)

type alertManagerMaintenance struct {
	client alertManagerSilenceClient
}

func newAlertManagerMaintenance(client client.Client) (Maintenance, error) {
	transport, err := getTransport(client)
	if err != nil {
		return nil, err
	}

	transport.DefaultAuthentication, err = getAuthentication(client)
	if err != nil {
		return nil, err
	}

	return &alertManagerMaintenance{
		client: alertManagerSilenceClient{
			transport: transport,
		},
	}, nil
}

func getTransport(c client.Client) (*httptransport.Runtime, error) {
	amRoute := &routev1.Route{}
	err := c.Get(
		context.TODO(),
		types.NamespacedName{Namespace: alertManagerNamespace, Name: alertManagerRouteName},
		amRoute,
	)
	if err != nil {
		return nil, err
	}

	return httptransport.New(
		amRoute.Spec.Host,
		alertManagerBasePath,
		[]string{"https"},
	), nil
}

func getAuthentication(c client.Client) (runtime.ClientAuthInfoWriter, error) {
	sl := &corev1.SecretList{}
	err := c.List(
		context.TODO(),
		sl,
		&client.ListOptions{Namespace: alertManagerNamespace},
	)
	if err != nil {
		return nil, err
	}

	var token string
	for _, s := range sl.Items {
		if strings.Contains(s.Name, alertManagerServiceAccountName+"-token") {
			token = string(s.Data["token"])
		}
	}

	return httptransport.BearerToken(token), nil
}

// Start a maintenance in Alertmanager
func (amm *alertManagerMaintenance) Start(endsAt time.Time) error {
	isRegex := true
	alertMatchKey := "alertname"
	// TODO: refine these alerts -> https://github.com/openshift/managed-upgrade-operator/pull/10#discussion_r434249118
	alertRegex := "[A-Z].*"
	silenceComment := "Silence for OSD upgrade"
	matchers := amv2Models.Matchers{
		{
			Name:    &alertMatchKey,
			IsRegex: &isRegex,
			Value:   &alertRegex,
		},
	}
	return amm.client.create(matchers, strfmt.DateTime(time.Now().UTC()), strfmt.DateTime(endsAt), config.OperatorName, silenceComment)
}

// End all active maintenances created by managed-upgrade-operator in Alertmanager
func (amm *alertManagerMaintenance) End() error {
	silences, err := amm.client.List([]string{})
	if err != nil {
		return err
	}

	var deleteErrors multierror.Error
	for _, s := range silences.Payload {
		if *s.CreatedBy == config.OperatorName && *s.Status.State == amv2Models.SilenceStatusStateActive {
			err := amm.client.Delete(*s.ID)
			if err != nil {
				deleteErrors = append(deleteErrors, err)
			}
		}
	}
	if len(deleteErrors) > 0 {
		return deleteErrors.AsError()
	}

	return nil
}

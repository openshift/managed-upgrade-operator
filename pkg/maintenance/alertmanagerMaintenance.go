package maintenance

import (
	"context"
	"github.com/hashicorp/go-multierror"
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

	// Generally upgrades should not fire critical alerts but there are some critical alerts that will fire.
	// e.g. 'etcdMembersDown' happens as the masters drain/reboot and a master is offline but this is expected and will resolve.
	// This is a regex of critical alerts that can be ignored while upgrading of controlplane occurs
	controlPlaneIgnoredCriticalAlerts = "(etcdMembersDown)"
)

type alertManagerMaintenanceBuilder struct{}

func (ammb *alertManagerMaintenanceBuilder) NewClient(client client.Client) (Maintenance, error) {
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

type alertManagerMaintenance struct {
	client alertManagerSilenceClient
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

// Start a control plane maintenance in Alertmanager
// Time is converted to UTC
func (amm *alertManagerMaintenance) StartControlPlane(endsAt time.Time) error {
	now := strfmt.DateTime(time.Now().UTC())
	end := strfmt.DateTime(endsAt.UTC())
	err := amm.client.create(createDefaultMatchers(), now, end, config.OperatorName, "Silence for OSD upgrade")
	if err != nil {
		return err
	}

	matchers := []*amv2Models.Matcher{createMatcher("alertname", controlPlaneIgnoredCriticalAlerts, true)}
	err = amm.client.create(matchers, now, end, config.OperatorName, "Silence for OSD upgrade")
	if err != nil {
		return err
	}

	return nil
}

// Start a control plane maintenance in Alertmanager
// Time is converted to UTC
func (amm *alertManagerMaintenance) StartWorker(endsAt time.Time) error {
	now := strfmt.DateTime(time.Now().UTC())
	end := strfmt.DateTime(endsAt.UTC())
	err := amm.client.create(createDefaultMatchers(), now, end, config.OperatorName, "Silence for OSD upgrade")
	if err != nil {
		return err
	}

	return nil
}

// End all active maintenances created by managed-upgrade-operator in Alertmanager
func (amm *alertManagerMaintenance) End() error {
	silences, err := amm.client.List([]string{})
	if err != nil {
		return err
	}

	var deleteErrors *multierror.Error
	for _, s := range silences.Payload {
		if *s.CreatedBy == config.OperatorName && *s.Status.State == amv2Models.SilenceStatusStateActive {
			err := amm.client.Delete(*s.ID)
			if err != nil {
				deleteErrors = multierror.Append(deleteErrors, err)
			}
		}
	}
	return deleteErrors.ErrorOrNil()
}

func createMatcher(alertMatchKey string, alertValue string, isRegex bool) *amv2Models.Matcher {
	return &amv2Models.Matcher{
		Name:    &alertMatchKey,
		IsRegex: &isRegex,
		Value:   &alertValue,
	}
}

func createDefaultMatchers() []*amv2Models.Matcher {
	// Upgrades can impact some availability which may trigger info/warning alerts. ignore those.
	nonCriticalAlertMatcher := createMatcher("severity", "(warning|info|none)", true)

	inNamespaceAlertMatcher := createMatcher("namespace", "(^openshift.*|^kube.*|^redhat.*|^default$)", true)
	return amv2Models.Matchers{nonCriticalAlertMatcher, inNamespaceAlertMatcher}
}

package maintenance

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/hashicorp/go-multierror"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/config"
	"github.com/openshift/managed-upgrade-operator/pkg/alertmanager"
	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
)

var (
	alertManagerApp                = "alertmanager-main"
	alertManagerServiceAccountName = "prometheus-k8s"
	alertManagerBasePath           = "/api/v2/"
	controlPlaneSilenceCommentId   = "OSD control plane"
	workerSilenceCommentId         = "OSD worker node"
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

	useRoutes := config.UseRoutes()
	tlsConfig := &tls.Config{}

	if !useRoutes {
		tlsConfig, err = metrics.MonitoringTLSConfig(client)
		if err != nil {
			return nil, err
		}
	}

	transport.Transport = &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &alertManagerMaintenance{
		client: &alertmanager.AlertManagerSilenceClient{
			Transport: transport,
		},
	}, nil
}

type alertManagerMaintenance struct {
	//	client alertManagerSilenceClient
	client alertmanager.AlertManagerSilencer
}

func getTransport(c client.Client) (*httptransport.Runtime, error) {
	networkTarget, err := metrics.NetworkTarget(c, metrics.MonitoringNS, alertManagerApp, "web")
	if err != nil {
		return nil, err
	}

	return httptransport.New(
		networkTarget,
		alertManagerBasePath,
		[]string{"https"},
	), nil
}

func getAuthentication(c client.Client) (runtime.ClientAuthInfoWriter, error) {
	sl := &corev1.SecretList{}
	err := c.List(
		context.TODO(),
		sl,
		&client.ListOptions{Namespace: metrics.MonitoringNS},
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

// Start a control plane maintenance in Alertmanager for version
// Time is converted to UTC
func (amm *alertManagerMaintenance) StartControlPlane(endsAt time.Time, version string, ignoredCriticalAlerts []string) error {
	defaultComment := fmt.Sprintf("Silence for %s upgrade to version %s", controlPlaneSilenceCommentId, version)
	defaultSilence, err := amm.client.Filter(equalsComment(defaultComment))
	if err != nil {
		return err
	}
	defaultExists := len(*defaultSilence) > 0

	criticalAlertComment := fmt.Sprintf("Silence for critical alerts during %s upgrade to version %s", controlPlaneSilenceCommentId, version)
	criticalSilence, err := amm.client.Filter(equalsComment(criticalAlertComment))
	if err != nil {
		return err
	}
	criticalExists := len(*criticalSilence) > 0

	if defaultExists && criticalExists {
		return nil
	}

	now := strfmt.DateTime(time.Now().UTC())
	end := strfmt.DateTime(endsAt.UTC())
	if !defaultExists {
		err = amm.client.Create(createDefaultMatchers(), now, end, config.OperatorName, defaultComment)
		if err != nil {
			return err
		}
	}

	if !criticalExists {
		if len(ignoredCriticalAlerts) > 0 {
			icRegex := "(" + strings.Join(ignoredCriticalAlerts, "|") + ")"
			matchers := []*amv2Models.Matcher{createMatcher("alertname", icRegex, true)}
			err = amm.client.Create(matchers, now, end, config.OperatorName, criticalAlertComment)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Start a worker node maintenance in Alertmanager for version
// Time is converted to UTC
func (amm *alertManagerMaintenance) SetWorker(endsAt time.Time, version string, count int32) error {
	comment := fmt.Sprintf("Silence for %s upgrade to version %s", workerSilenceCommentId, version)
	fullComment := fmt.Sprintf("%s with remaining %d nodes", comment, count)
	silenceList, err := amm.client.Filter(equalsComment(fullComment))
	if err != nil {
		return err
	}

	exists := len(*silenceList) > 0

	end := strfmt.DateTime(endsAt.UTC())
	if !exists {
		oldSilenceList, err := amm.client.Filter(activeSilences, containsComment(comment))
		if err != nil {
			return err
		}
		if len(*oldSilenceList) > 0 {
			oldSl := *oldSilenceList
			oldSilence := oldSl[0]
			err = amm.client.Delete(*oldSilence.ID)
			if err != nil {
				return err
			}
		}
		now := strfmt.DateTime(time.Now().UTC())
		err = amm.client.Create(createDefaultMatchers(), now, end, config.OperatorName, fullComment)
		if err != nil {
			return err
		}
	}
	return nil
}

// End all active control plane maintenances created by managed-upgrade-operator in Alertmanager
func (amm *alertManagerMaintenance) EndControlPlane() error {
	return amm.EndSilences(controlPlaneSilenceCommentId)
}

// End all active worker maintenances created by managed-upgrade-operator in Alertmanager
func (amm *alertManagerMaintenance) EndWorker() error {
	return amm.EndSilences(workerSilenceCommentId)
}

// End all active control plane maintenances created by managed-upgrade-operator in Alertmanager
// that have a comment field containing the supplied value
func (amm *alertManagerMaintenance) EndSilences(comment string) error {
	silences, err := amm.client.Filter(createdByOperator, activeSilences, containsComment(comment))
	if err != nil {
		return err
	}

	var deleteErrors *multierror.Error
	for _, s := range *silences {
		err := amm.client.Delete(*s.ID)
		if err != nil {
			deleteErrors = multierror.Append(deleteErrors, err)
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
	nonCriticalAlertMatcher := createMatcher("severity", "(warning|info)", true)

	inNamespaceAlertMatcher := createMatcher("namespace", "(^openshift.*|^kube.*|^redhat.*|^default$)", true)
	return amv2Models.Matchers{nonCriticalAlertMatcher, inNamespaceAlertMatcher}
}

func (amm *alertManagerMaintenance) IsActive() (bool, error) {
	silences, err := amm.client.Filter(activeSilences, createdByOperator)
	if err != nil {
		return false, err
	}

	return len(*silences) > 0, nil
}

var activeSilences = func(s *amv2Models.GettableSilence) bool {
	return *s.Status.State == amv2Models.AlertStatusStateActive
}

var createdByOperator = func(s *amv2Models.GettableSilence) bool {
	return *s.CreatedBy == config.OperatorName
}

var equalsComment = func(comment string) func(s *amv2Models.GettableSilence) bool {
	return func(s *amv2Models.GettableSilence) bool {
		return *s.Comment == comment
	}
}

var containsComment = func(comment string) func(s *amv2Models.GettableSilence) bool {
	return func(s *amv2Models.GettableSilence) bool {
		return strings.Contains(*s.Comment, comment)
	}
}

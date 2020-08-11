package maintenance

import (
	"context"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	amSilence "github.com/prometheus/alertmanager/api/v2/client/silence"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
	"net/http"
)

//go:generate mockgen -destination=alertManagerSilenceMock.go -package=maintenance github.com/openshift/managed-upgrade-operator/pkg/maintenance AlertManagerSilencer
type AlertManagerSilencer interface {
	Create(matchers amv2Models.Matchers, startsAt strfmt.DateTime, endsAt strfmt.DateTime, creator string, comment string) error
	List(filter []string) (*amSilence.GetSilencesOK, error)
	Delete(id string) error
	Update(id string, endsAt strfmt.DateTime) error
}

type alertManagerSilenceClient struct {
	transport *httptransport.Runtime
}

// Creates a silence in Alertmanager instance defined in transport
func (ams *alertManagerSilenceClient) Create(matchers amv2Models.Matchers, startsAt strfmt.DateTime, endsAt strfmt.DateTime, creator string, comment string) error {
	pParams := &amSilence.PostSilencesParams{
		Silence: &amv2Models.PostableSilence{
			Silence: amv2Models.Silence{
				CreatedBy: &creator,
				Comment:   &comment,
				EndsAt:    &endsAt,
				StartsAt:  &startsAt,
				Matchers:  matchers,
			},
		},
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}

	silenceClient := amSilence.New(ams.transport, strfmt.Default)
	_, err := silenceClient.PostSilences(pParams)
	if err != nil {
		return err
	}
	return nil
}

// list silences in Alertmanager instance defined in transport
func (ams *alertManagerSilenceClient) List(filter []string) (*amSilence.GetSilencesOK, error) {
	gParams := &amSilence.GetSilencesParams{
		Filter:     filter,
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}

	silenceClient := amSilence.New(ams.transport, strfmt.Default)
	results, err := silenceClient.GetSilences(gParams)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// Delete silence in Alertmanager instance defined in transport
func (ams *alertManagerSilenceClient) Delete(id string) error {
	dParams := &amSilence.DeleteSilenceParams{
		SilenceID:  strfmt.UUID(id),
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}

	silenceClient := amSilence.New(ams.transport, strfmt.Default)
	_, err := silenceClient.DeleteSilence(dParams)
	if err != nil {
		return err
	}

	return nil
}

// Update silence end time in AlertManager instance defined in transport
func (ams *alertManagerSilenceClient) Update(id string, endsAt strfmt.DateTime) error {
	silenceClient := amSilence.New(ams.transport, strfmt.Default)
	gParams := &amSilence.GetSilenceParams{
		SilenceID:  strfmt.UUID(id),
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}
	result, err := silenceClient.GetSilence(gParams)
	if err != nil {
		return err
	}

	pParams := &amSilence.PostSilencesParams{
		Silence: &amv2Models.PostableSilence{
			ID: id,
			Silence: amv2Models.Silence{
				CreatedBy: result.Payload.CreatedBy,
				Comment:   result.Payload.Comment,
				EndsAt:    &endsAt,
				StartsAt:  result.Payload.StartsAt,
				Matchers:  result.Payload.Matchers,
			},
		},
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}

	_, err = silenceClient.PostSilences(pParams)
	if err != nil {
		return err
	}

	return nil
}

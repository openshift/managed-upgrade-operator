package alertmanager

import (
	"context"
	"fmt"
	"net/http"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	amSilence "github.com/prometheus/alertmanager/api/v2/client/silence"
	amv2Models "github.com/prometheus/alertmanager/api/v2/models"
)

// AlertManagerSilencer interface enables implementations of an AlertManagerSilencer
//go:generate mockgen -destination=mocks/alertManagerSilenceClient.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/alertmanager AlertManagerSilencer
type AlertManagerSilencer interface {
	Create(matchers amv2Models.Matchers, startsAt strfmt.DateTime, endsAt strfmt.DateTime, creator string, comment string) error
	List(filter []string) (*amSilence.GetSilencesOK, error)
	Delete(id string) error
	Update(id string, endsAt strfmt.DateTime) error
	Filter(predicates ...SilencePredicate) (*[]amv2Models.GettableSilence, error)
}

// AlertManagerSilenceClient holds fields for an AlertManagerSilenceClient
type AlertManagerSilenceClient struct {
	Transport *httptransport.Runtime
}

// Create creates a silence in Alertmanager instance defined in Transport
func (ams *AlertManagerSilenceClient) Create(matchers amv2Models.Matchers, startsAt strfmt.DateTime, endsAt strfmt.DateTime, creator string, comment string) error {
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

	silenceClient := amSilence.New(ams.Transport, strfmt.Default)
	_, err := silenceClient.PostSilences(pParams)
	if err != nil {
		return err
	}

	return nil
}

// List lists silences in Alertmanager instance defined in Transport
func (ams *AlertManagerSilenceClient) List(filter []string) (*amSilence.GetSilencesOK, error) {
	gParams := &amSilence.GetSilencesParams{
		Filter:     filter,
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}

	silenceClient := amSilence.New(ams.Transport, strfmt.Default)
	results, err := silenceClient.GetSilences(gParams)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// Delete deletes silence in Alertmanager instance defined in Transport
func (ams *AlertManagerSilenceClient) Delete(id string) error {
	dParams := &amSilence.DeleteSilenceParams{
		SilenceID:  strfmt.UUID(id),
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}

	silenceClient := amSilence.New(ams.Transport, strfmt.Default)
	_, err := silenceClient.DeleteSilence(dParams)
	if err != nil {
		return err
	}

	return nil
}

// Update updates silence end time in AlertManager instance defined in Transport
func (ams *AlertManagerSilenceClient) Update(id string, endsAt strfmt.DateTime) error {
	silenceClient := amSilence.New(ams.Transport, strfmt.Default)
	gParams := &amSilence.GetSilenceParams{
		SilenceID:  strfmt.UUID(id),
		Context:    context.TODO(),
		HTTPClient: &http.Client{},
	}
	result, err := silenceClient.GetSilence(gParams)
	if err != nil {
		return err
	}

	// Create a new silence first
	err = ams.Create(result.Payload.Matchers, *result.Payload.StartsAt, endsAt, *result.Payload.CreatedBy, *result.Payload.Comment)
	if err != nil {
		return fmt.Errorf("unable to create replacement silence: %v", err)
	}

	// Remove the old silence if it's still active
	if *result.Payload.Status.State == amv2Models.SilenceStatusStateActive {
		err = ams.Delete(*result.Payload.ID)
		if err != nil {
			return fmt.Errorf("unable to remove replaced silence: %v", err)
		}
	}

	return nil
}

// SilencePredicate is a predicate that returns a bool
type SilencePredicate func(*amv2Models.GettableSilence) bool

// Filter filters silences in Alertmanager based on the predicates
func (ams *AlertManagerSilenceClient) Filter(predicates ...SilencePredicate) (*[]amv2Models.GettableSilence, error) {
	silences, err := ams.List([]string{})
	if err != nil {
		return nil, err
	}

	filteredSilences := []amv2Models.GettableSilence{}
	for _, s := range silences.Payload {
		var match = true
		for _, p := range predicates {
			if !p(s) {
				match = false
				break
			}
		}
		if match {
			filteredSilences = append(filteredSilences, *s)
		}
	}

	return &filteredSilences, nil
}

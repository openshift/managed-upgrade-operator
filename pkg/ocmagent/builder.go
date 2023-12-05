package ocmagent

import (
	"net/url"
	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
)

//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClientBuilder
// OcmAgentClientBuilder enables implementation of an ocm client.
type OcmAgentClientBuilder interface {
	New(ocmBaseUrl *url.URL) (ocm.OcmClient, error)
}

// NewBuilder creates a new Notifier instance builder
func NewBuilder() OcmAgentClientBuilder {
	return &ocmAgentClientBuilder{}
}

type ocmAgentClientBuilder struct{}

func (oacb *ocmAgentClientBuilder) New(ocmBaseUrl *url.URL) (ocm.OcmClient, error) {
	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&ocmRoundTripper{})

	return &ocmClient{
		ocmBaseUrl: ocmBaseUrl,
		httpClient: httpClient,
	}, nil
}

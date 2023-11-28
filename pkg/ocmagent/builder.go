package ocmagent

import (
	"net/url"
	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClientBuilder
// OcmAgentClientBuilder enables implementation of an ocm client.
type OcmAgentClientBuilder interface {
	New(c client.Client, ocmBaseUrl *url.URL) (ocm.OcmClient, error)
}

// NewBuilder creates a new Notifier instance builder
func NewBuilder() OcmAgentClientBuilder {
	return &ocmAgentClientBuilder{}
}

type ocmAgentClientBuilder struct{}

func (oacb *ocmAgentClientBuilder) New(c client.Client, ocmBaseUrl *url.URL) (ocm.OcmClient, error) {
	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&ocmRoundTripper{})

	return &ocmClient{
		client:     c,
		ocmBaseUrl: ocmBaseUrl,
		httpClient: httpClient,
	}, nil
}

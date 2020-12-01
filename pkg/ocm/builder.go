package ocm

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/util"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClientBuilder
type OcmClientBuilder interface {
	New(c client.Client, ocmBaseUrl *url.URL) (OcmClient, error)
}

// Creates a new Notifier instance builder
func NewBuilder() OcmClientBuilder {
	return &ocmClientBuilder{}
}

type ocmClientBuilder struct{}

func (ocb *ocmClientBuilder) New(c client.Client, ocmBaseUrl *url.URL) (OcmClient, error) {

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(c)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cluster access token")
	}

	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&ocmRoundTripper{authorization: *accessToken})

	return &ocmClient{
		client:     c,
		ocmBaseUrl: ocmBaseUrl,
		httpClient: httpClient,
	}, nil

}

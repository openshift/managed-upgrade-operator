package dvo

import (
	"fmt"
	"net/url"

	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DvoClientBuilder enables implementation of a DVO client.
//
//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/dvo DvoClientBuilder
type DvoClientBuilder interface {
	New(c client.Client, dvoURL *url.URL) (DvoClient, error)
}

// NewBuilder creates a new DvoClientBuilder instance
func NewBuilder() DvoClientBuilder {
	return &dvoClientBuilder{}
}

type dvoClientBuilder struct{}

func (dcb *dvoClientBuilder) New(c client.Client, dvoURL *url.URL) (DvoClient, error) {

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(c)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cluster access token")
	}

	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&dvoRoundTripper{authorization: *accessToken})

	return &dvoClient{
		client:     c,
		dvoBaseUrl: dvoURL,
		httpClient: httpClient,
	}, nil

}


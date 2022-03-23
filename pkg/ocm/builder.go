package ocm

import (
	"fmt"
	"net/url"
	"os"

	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClientBuilder
// OcmClientBuilder enables implementation of an ocm client.
type OcmClientBuilder interface {
	New(c client.Client, ocmBaseUrl *url.URL) (OcmClient, error)
}

// NewBuilder creates a new Notifier instance builder
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

	// Inherit environment proxy config
	var proxyURL *url.URL
	proxy := getProxy()
	if proxy != "" {
		proxyURL, err = url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid-formatted proxy: %v", err)
		}
	}

	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&ocmRoundTripper{authorization: *accessToken, proxy: proxyURL})

	return &ocmClient{
		client:     c,
		ocmBaseUrl: ocmBaseUrl,
		httpClient: httpClient,
	}, nil

}

func getProxy() string {
	// Default to HTTPS_PROXY if available
	httpsProxy := os.Getenv("HTTPS_PROXY")
	if len(httpsProxy) > 0  {
		return httpsProxy
	}
	return ""
}

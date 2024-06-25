package ocm

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/go-resty/resty/v2"
	"github.com/openshift/managed-upgrade-operator/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sdk "github.com/openshift-online/ocm-sdk-go"
)

// OcmClientBuilder enables implementation of an ocm client.
//
//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClientBuilder
type OcmClientBuilder interface {
	New(c client.Client, ocmBaseUrl *url.URL) (OcmClient, error)
}

// NewBuilder creates a new Notifier instance builder
func NewBuilder() OcmClientBuilder {
	return &ocmClientBuilder{}
}

type ocmClientBuilder struct{}

// SdkClient is the ocm client with which we can run the commands
// currently we do not need to export the connection or the config, as we create the SdkClient using the New func
type SdkClient struct {
	conn *sdk.Connection
}

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

	// Setup OCM SDK client using the token
	authVal := fmt.Sprintf("%v:%v", accessToken.ClusterId, accessToken.PullSecret)
	sdkConnection, err := sdk.NewConnectionBuilder().Tokens(authVal).URL(ocmBaseUrl.String()).BuildContext(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't build connection: %v\n", err)
		return nil, err
	}

	client := SdkClient{}
	client.conn = sdkConnection

	return &ocmClient{
		client:     c,
		ocmBaseUrl: ocmBaseUrl,
		httpClient: httpClient,
		sdkClient:  &client,
	}, nil

}

func getProxy() string {
	// Default to HTTPS_PROXY if available
	httpsProxy := os.Getenv("HTTPS_PROXY")
	if len(httpsProxy) > 0 {
		return httpsProxy
	}
	return ""
}

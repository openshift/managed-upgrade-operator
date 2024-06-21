package ocmagent

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/go-resty/resty/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/managed-upgrade-operator/pkg/ocm"
	"github.com/openshift/managed-upgrade-operator/util"
)

// OcmAgentClientBuilder enables implementation of an ocm client.
//
//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/ocm OcmClientBuilder
type OcmAgentClientBuilder interface {
	New(c client.Client, ocmBaseUrl *url.URL) (ocm.OcmClient, error)
}

// NewBuilder creates a new Notifier instance builder
func NewBuilder() OcmAgentClientBuilder {
	return &ocmAgentClientBuilder{}
}

type ocmAgentClientBuilder struct{}

// SdkClient is the ocm client with which we can run the commands
// currently we do not need to export the connection or the config, as we create the SdkClient using the New func
type SdkClient struct {
	conn *sdk.Connection
}

func (oacb *ocmAgentClientBuilder) New(c client.Client, ocmBaseUrl *url.URL) (ocm.OcmClient, error) {

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(c)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cluster access token")
	}

	// Set up the HTTP client using the token
	httpClient := resty.New().SetTransport(&ocmRoundTripper{})

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

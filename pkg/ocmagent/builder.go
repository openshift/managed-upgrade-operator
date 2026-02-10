package ocmagent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/managed-upgrade-operator/config"
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

	// Setup OCM SDK client with custom auth and TLS timeout (no proxy for local service)
	sdkConnection, err := sdk.NewConnectionBuilder().
		URL(ocmBaseUrl.String()).
		Agent(config.SetUserAgent()).
		TransportWrapper(func(base http.RoundTripper) http.RoundTripper {
			return &ocmAgentAuthTransport{
				wrapped:       base,
				authorization: *accessToken,
			}
		}).
		BuildContext(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't build connection: %v\n", err)
		return nil, err
	}

	return &ocmClient{
		client:     c,
		ocmBaseUrl: ocmBaseUrl,
		conn:       sdkConnection,
	}, nil

}

// ocmAgentAuthTransport is a custom HTTP transport for OCM Agent service
// that adds authentication and configures TLS timeout (no proxy needed for local service)
type ocmAgentAuthTransport struct {
	wrapped       http.RoundTripper
	authorization util.AccessToken
}

func (t *ocmAgentAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add custom authorization header
	authVal := fmt.Sprintf("AccessToken %s:%s", t.authorization.ClusterId, t.authorization.PullSecret)
	req.Header.Set("Authorization", authVal)

	// Configure TLS timeout if the base transport is *http.Transport
	if transport, ok := t.wrapped.(*http.Transport); ok {
		transport.TLSHandshakeTimeout = 5 * time.Second
	}

	return t.wrapped.RoundTrip(req)
}

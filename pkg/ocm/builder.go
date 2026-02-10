package ocm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/openshift/managed-upgrade-operator/config"
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

	// Setup OCM SDK client with token, retry, and timeout configuration
	sdkConnection, err := sdk.NewConnectionBuilder().
		URL(ocmBaseUrl.String()).
		Agent(config.SetUserAgent()).
		Tokens(fmt.Sprintf("%s:%s", accessToken.ClusterId, accessToken.PullSecret)).

		// Retry configuration: SDK will retry on 503, 429, and network errors
		RetryLimit(5).                  // Maximum 5 retry attempts (default: 2)
		RetryInterval(2 * time.Second). // Initial retry delay of 2 seconds (default: 1s)
		RetryJitter(0.3).               // 30% jitter to avoid thundering herd (default: 0.2)

		// Transport wrapper for proxy and timeout configuration
		TransportWrapper(func(base http.RoundTripper) http.RoundTripper {
			if transport, ok := base.(*http.Transport); ok {
				// Configure proxy using Go's standard environment variable handling
				// Respects HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables
				// See: https://pkg.go.dev/net/http#ProxyFromEnvironment
				transport.Proxy = http.ProxyFromEnvironment

				// Configure timeouts for reliable OCM API communication
				transport.DialContext = (&net.Dialer{
					Timeout:   30 * time.Second, // Maximum time to establish TCP connection
					KeepAlive: 30 * time.Second, // TCP keep-alive probe interval
				}).DialContext
				transport.TLSHandshakeTimeout = 10 * time.Second // Maximum time for TLS handshake
			}
			return base
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

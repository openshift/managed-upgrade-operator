package dvo

import (
	"fmt"
	"net/http"

	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"github.com/openshift/managed-upgrade-operator/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DvoClientBuilder enables implementation of a DVO client.
//go:generate mockgen -destination=mocks/builder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/dvo DvoClientBuilder
type DvoClientBuilder interface {
	New(c client.Client) (DvoClient, error)
}

// NewBuilder creates a new DvoClientBuilder instance
func NewBuilder() DvoClientBuilder {
	return &dvoClientBuilder{}
}

type dvoClientBuilder struct{}

// New creates a new instance of DvoClient.
// It takes a client.Client as input and returns a DvoClient interface and an error.
func (dcb *dvoClientBuilder) New(c client.Client) (DvoClient, error) {

	// Get the service URL for the deployment-validation-operator-metrics service
	svcURL, err := metrics.NetworkTarget(c, "openshift-deployment-validation-operator", "deployment-validation-operator-metrics", "8383")
	if err != nil {
		return nil, err
	}

	// Print the service URL for debugging purposes
	fmt.Printf("********%s", svcURL)

	// Fetch the cluster AccessToken
	accessToken, err := util.GetAccessToken(c)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve cluster access token")
	}

	// Set up the HTTP client using the token
	httpClient := http.Client{
		Transport: &dvoRoundTripper{authorization: *accessToken},
	}

	// Create and return a new instance of dvoClient
	return &dvoClient{
		client:     c,
		dvoBaseUrl: svcURL,
		httpClient: httpClient,
	}, nil

}

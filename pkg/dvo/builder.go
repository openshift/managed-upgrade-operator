package dvo

import (
	"net/http"
	"os"
	"time"

	"github.com/openshift/managed-upgrade-operator/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DvoClientBuilder enables implementation of a DVO client.
//
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
	svcURL, err := metrics.NetworkTarget(c, "openshift-deployment-validation-operator", "deployment-validation-operator-metrics", "http-metrics")
	// Override svcURL for DVO, when environment variable is set
	dvoSVCURL := os.Getenv("DVO_SVC_URL")
	if dvoSVCURL != "" {
		svcURL = dvoSVCURL
	}

	if err != nil {
		return nil, err
	}

	httpClient := http.Client{
		Timeout:   30 * time.Second,
		Transport: dvoTransport(),
	}

	// Create and return a new instance of dvoClient
	return &dvoClient{
		client:     c,
		dvoBaseUrl: svcURL,
		httpClient: httpClient,
	}, nil

}

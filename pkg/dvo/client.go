package dvo

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openshift/managed-upgrade-operator/util"
)

const (

	// CLUSTERS_V1_PATH is a path to the OCM clusters service
	METRICS_API_PATH = "/metrics"
)

var log = logf.Log.WithName("dvo-client")

var (
	// ErrClusterIdNotFound is an error describing the cluster ID can not be found
	ErrClusterIdNotFound = fmt.Errorf("OCM did not return a valid cluster ID: pull-secret may be invalid OR cluster's owner is disabled/banned in OCM")
)

// DvoClient enables an implementation of a DVO client
//go:generate mockgen -destination=mocks/client.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/dvo DvoClient
type DvoClient interface {
	GetMetrics() error
}

type dvoClient struct {
	// Cluster k8s client
	client client.Client
	// Base DVO API Url
	dvoBaseUrl *url.URL
	// HTTP client used for API queries (TODO: remove in favour of DVO SDK)
	httpClient *resty.Client
}

type dvoRoundTripper struct {
	authorization util.AccessToken
}

func (drt *dvoRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	authVal := fmt.Sprintf("AccessToken %s:%s", drt.authorization.ClusterId, drt.authorization.PullSecret)
	req.Header.Add("Authorization", authVal)
	transport := http.Transport{
		TLSHandshakeTimeout: time.Second * 5,
	}
	return transport.RoundTrip(req)
}

func (c *dvoClient) GetMetrics() error {
	// Construct the URL for the metrics API
	metricsURL := c.dvoBaseUrl.String() + METRICS_API_PATH

	// Create a new HTTP request
	req, err := http.NewRequest(http.MethodGet, metricsURL, nil)
	if err != nil {
		return err
	}

	// Send the HTTP request
	resp, err := c.httpClient.R().Execute(req.Method, req.URL.String())
	fmt.Println("*************")
	fmt.Print(resp)
	if err != nil {
		return err
	}
	defer func() {
		if resp != nil && resp.RawResponse != nil && resp.RawResponse.Body != nil {
			resp.RawResponse.Body.Close()
		}
	}()

	// Check the response status code
	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("failed to get metrics: %s", resp.Status())
	}

	// TODO: Process the response body

	return nil
}

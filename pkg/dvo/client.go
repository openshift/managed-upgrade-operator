package dvo

import (
	"fmt"
	"io"
	"net/http"
	"time"

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
	GetMetrics() ([]byte, error)
}

type dvoClient struct {
	// Cluster k8s client
	client client.Client
	// Base DVO API Url
	dvoBaseUrl string
	// HTTP client used for API queries (TODO: remove in favour of DVO SDK)
	httpClient http.Client
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

func (c *dvoClient) GetMetrics() ([]byte, error) {
	// Construct the URL for the metrics API
	metricsURL := "http://" + c.dvoBaseUrl + METRICS_API_PATH

	req, err := http.NewRequest("GET", metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not query prometheus: %s", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error when querying prometheus: %s", err)
	}

	if resp != nil {
		fmt.Printf("********%s", body)
	}

	return body, nil

}

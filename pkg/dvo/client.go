package dvo

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// CLUSTERS_V1_PATH is a path to the OCM clusters service
	METRICS_API_PATH = "/metrics"
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

func newDvoTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 30 * time.Second,
	}
}

func (c *dvoClient) GetMetrics() ([]byte, error) {
	// Construct the URL for the metrics API
	metricsURL := "http://" + c.dvoBaseUrl + METRICS_API_PATH

	req, err := http.NewRequest("GET", metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not query DVO metrics endpoint: %v", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error when querying dvo endpoint: %v", err)
	}
	return body, nil

}

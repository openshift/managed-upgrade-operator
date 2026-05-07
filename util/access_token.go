package util

import (
	"context"
	"encoding/json"
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pullSecretKey     = ".dockerconfigjson" //#nosec G101 -- This is a false positive
	pullSecretAuthKey = "cloud.openshift.com"
)

// AccessToken contains fields for an access token
type AccessToken struct {
	PullSecret string
	ClusterId  string
}

// GetAccessToken fetches the access token for authentication to the Cluster Service API via the cluster pull secret
func GetAccessToken(c client.Client) (*AccessToken, error) {

	cv := &configv1.ClusterVersion{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: "version"}, cv)
	if err != nil {
		return nil, fmt.Errorf("can't get clusterversion: %w", err)
	}

	secret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch pull secret: %w", err)
	}

	pullSecret, ok := secret.Data[pullSecretKey]
	if !ok {
		return nil, fmt.Errorf("pull secret missing required key %v", pullSecretKey)
	}

	var dockerConfig map[string]interface{}
	err = json.Unmarshal(pullSecret, &dockerConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to interpret decoded pull secret as json: %w", err)
	}
	authConfig, ok := dockerConfig["auths"]
	if !ok {
		return nil, fmt.Errorf("unable to find auths section in pull secret")
	}
	authMap, ok := authConfig.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("auths section is not a map")
	}
	apiConfig, ok := authMap[pullSecretAuthKey]
	if !ok {
		return nil, fmt.Errorf("unable to find pull secret auth key '%s' in pull secret", pullSecretAuthKey)
	}
	apiMap, ok := apiConfig.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("api config is not a map")
	}
	accessToken, ok := apiMap["auth"]
	if !ok {
		return nil, fmt.Errorf("unable to find access auth token in pull secret")
	}
	strAccessToken := fmt.Sprintf("%v", accessToken)

	at := &AccessToken{
		ClusterId:  string(cv.Spec.ClusterID),
		PullSecret: strAccessToken,
	}

	return at, nil
}

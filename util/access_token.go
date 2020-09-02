package util

import (
	"context"
	"encoding/json"
	"fmt"
	"encoding/base64"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pullSecretKey = ".dockerconfigjson"
	pullSecretAuthKey = "cloud.openshift.com"
)

// Fetches the access token for authentication to the Cluster Service API via the cluster pull secret
func GetAccessToken(c client.Client) (*string, error) {

	secret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: "openshift-config", Name: "pull-secret"}, secret)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch pull secret: %v", err)
	}

	encodedSecret, ok := secret.Data[pullSecretKey]
	if !ok {
		return nil, fmt.Errorf("pull secret missing required key %v", pullSecretKey)
	}
	decodedSecret, err := base64.StdEncoding.DecodeString(string(encodedSecret))
	if err != nil {
		return nil, fmt.Errorf("unable to decode pull secret: %v", err)
	}

	var dockerConfig map[string]interface{}
	err = json.Unmarshal(decodedSecret, &dockerConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to interpret decoded pull secret as json: %v", err)
	}
	authConfig, ok := dockerConfig["auths"]
	if !ok {
		return nil, fmt.Errorf("unable to find auths section in pull secret")
	}
	apiConfig, ok := authConfig.(map[string]interface{})[pullSecretAuthKey]
	if !ok {
		return nil, fmt.Errorf("unable to find pull secret auth key '%s' in pull secret", pullSecretAuthKey)
	}
	accessToken, ok := apiConfig.(map[string]interface{})["auth"]
	if !ok {
		return nil, fmt.Errorf("unable to find access auth token in pull secret")
	}
	strAccessToken := fmt.Sprintf("%v", accessToken)
	return &strAccessToken, nil
}

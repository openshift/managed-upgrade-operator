package configmanager

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/config"
)

const (
	// CONFIG_MAP_NAME is the name of the operators config
	CONFIG_MAP_NAME = config.OperatorName + "-config"
	// CONFIG_PATH is the name of the config
	CONFIG_PATH = "config.yaml"
)

// ConfigManagerBuilder is an interface describing the functions of a cluster upgrader.
//go:generate mockgen -destination=mocks/configmanagerbuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/configmanager ConfigManagerBuilder
type ConfigManagerBuilder interface {
	New(client.Client, string) ConfigManager
}

type configManagerBuilder struct{}

func (*configManagerBuilder) New(c client.Client, ns string) ConfigManager {
	return &configManager{
		client:    c,
		namespace: ns,
	}
}

// NewBuilder returns a new configManagerBuilder
func NewBuilder() ConfigManagerBuilder {
	return &configManagerBuilder{}
}

// ConfigManager is an interface describing the functions of a cluster upgrader.
//go:generate mockgen -destination=mocks/configmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/configmanager ConfigManager
type ConfigManager interface {
	Into(ConfigValidator) error
}

type configManager struct {
	client    client.Client
	namespace string
}

// ConfigValidator is an interface that validates MUO's config
type ConfigValidator interface {
	IsValid() error
}

func (cm *configManager) Into(into ConfigValidator) error {
	cfgMap := &corev1.ConfigMap{}
	err := cm.client.Get(context.TODO(), client.ObjectKey{Name: CONFIG_MAP_NAME, Namespace: cm.namespace}, cfgMap)
	if err != nil {
		return err
	}
	yml := cfgMap.Data[CONFIG_PATH]
	err = yaml.Unmarshal([]byte(yml), into)
	if err != nil {
		return fmt.Errorf("Check config map %s for incorrect yaml formatting %s", CONFIG_MAP_NAME, err.Error())
	}

	return into.IsValid()
}

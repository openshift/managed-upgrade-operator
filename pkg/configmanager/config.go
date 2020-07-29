package configmanager

import (
	"context"
	"fmt"
	"github.com/openshift/managed-upgrade-operator/config"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	CONFIG_MAP_NAME = config.OperatorName + "-config"
	CONFIG_PATH     = "config.yaml"
)

// Interface describing the functions of a cluster upgrader.
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

func NewBuilder() ConfigManagerBuilder {
	return &configManagerBuilder{}
}

// Interface describing the functions of a cluster upgrader.
//go:generate mockgen -destination=mocks/configmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/configmanager ConfigManager
type ConfigManager interface {
	Into(ConfigValidator) error
}

type configManager struct {
	client    client.Client
	namespace string
}

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

package configmanager

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigManagerBuilder is an interface describing the functions of a cluster upgrader.
//
//go:generate mockgen -destination=mocks/configmanagerbuilder.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/configmanager ConfigManagerBuilder
type ConfigManagerBuilder interface {
	New(client.Client, Target) ConfigManager
}

type configManagerBuilder struct{}

func (*configManagerBuilder) New(c client.Client, t Target) ConfigManager {
	return &configManager{
		client: c,
		target: t,
	}
}

// NewBuilder returns a new configManagerBuilder
func NewBuilder() ConfigManagerBuilder {
	return &configManagerBuilder{}
}

// ConfigManager is an interface describing the functions of a cluster upgrader.
//
//go:generate mockgen -destination=mocks/configmanager.go -package=mocks github.com/openshift/managed-upgrade-operator/pkg/configmanager ConfigManager
type ConfigManager interface {
	Into(ConfigValidator) error
}

type Target struct {
	Name      string
	Namespace string
	ConfigKey string
}

type configManager struct {
	client client.Client
	target Target
}

// ConfigValidator is an interface that validates MUO's config
type ConfigValidator interface {
	IsValid() error
}

func (cm *configManager) Into(into ConfigValidator) error {
	cfgMap := &corev1.ConfigMap{}
	err := cm.client.Get(context.TODO(), client.ObjectKey{Name: cm.target.Name, Namespace: cm.target.Namespace}, cfgMap)
	if err != nil {
		return err
	}

	ck := cm.target.ConfigKey

	yml := cfgMap.Data[ck]
	err = yaml.Unmarshal([]byte(yml), into)
	if err != nil {
		return fmt.Errorf("check config map %s for incorrect yaml formatting %s", cm.target.Name, err.Error())
	}

	return into.IsValid()
}

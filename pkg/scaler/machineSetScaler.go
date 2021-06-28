package scaler

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/drain"
	"github.com/openshift/managed-upgrade-operator/pkg/machinery"
)

const (
	// LABEL_UPGRADE is the label used for managed upgrades
	LABEL_UPGRADE = "upgrade.managed.openshift.io"
	// LABEL_MACHINESET is the label used for machinesets
	LABEL_MACHINESET = "machine.openshift.io/cluster-api-machineset"
	// MACHINE_API_NAMESPACE is the namespace of the machine api
	MACHINE_API_NAMESPACE = "openshift-machine-api"
)

type machineSetScaler struct{}

// EnsureScaleUpNodes will create a new MachineSet with 1 extra replicas for workers in every region and report when the nodes are ready.
func (s *machineSetScaler) EnsureScaleUpNodes(c client.Client, timeOut time.Duration, logger logr.Logger) (bool, error) {
	upgradeMachinesets := &machineapi.MachineSetList{}

	err := c.List(context.TODO(), upgradeMachinesets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}
	originalMachineSets := &machineapi.MachineSetList{}

	err = c.List(context.TODO(), originalMachineSets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{"hive.openshift.io/machine-pool": "worker"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get original machinesets")
		return false, err
	}
	if len(originalMachineSets.Items) == 0 {
		logger.Info("failed to get machineset")
		return false, fmt.Errorf("failed to get original machineset")
	}

	updated := false
	for _, ms := range originalMachineSets.Items {

		found := false
		for _, ums := range upgradeMachinesets.Items {
			if ums.Name == ms.Name+"-upgrade" {
				found = true
			}
		}
		if found {
			logger.Info(fmt.Sprintf("machineset for upgrade already created :%s", ms.Name))
			continue
		}
		updated = true
		replica := int32(1)
		newMs := ms.DeepCopy()

		newMs.ObjectMeta = metav1.ObjectMeta{
			Name:      ms.Name + "-upgrade",
			Namespace: ms.Namespace,
			Labels: map[string]string{
				LABEL_UPGRADE: "true",
			},
		}
		newMs.Spec.Replicas = &replica
		newMs.Spec.Template.Labels[LABEL_UPGRADE] = "true"
		newMs.Spec.Template.Labels[LABEL_MACHINESET] = newMs.Name
		newMs.Spec.Selector.MatchLabels[LABEL_UPGRADE] = "true"
		newMs.Spec.Selector.MatchLabels[LABEL_MACHINESET] = newMs.Name
		logger.Info(fmt.Sprintf("creating machineset %s for upgrade", newMs.Name))

		err = c.Create(context.TODO(), newMs)
		if err != nil {
			logger.Error(err, "failed to create machineset")
			return false, err
		}

	}
	if updated {
		// New machineset created, machines must not ready at the moment, so skip following steps
		return false, nil
	}
	allNodeReady := true
	for _, ms := range upgradeMachinesets.Items {
		//We assume the create time is the start time for scale up extra compute nodes
		startTime := ms.CreationTimestamp
		if ms.Status.Replicas != ms.Status.ReadyReplicas {

			if time.Now().After(startTime.Time.Add(timeOut)) {
				return false, NewScaleTimeOutError(fmt.Sprintf("Machineset %s provisioning timout", ms.Name))
			}
			logger.Info(fmt.Sprintf("not all machines are ready for machineset:%s", ms.Name))
			return false, nil
		}

		machines := &machineapi.MachineList{}
		err := c.List(context.TODO(), machines, []client.ListOption{
			client.InNamespace(MACHINE_API_NAMESPACE),
			client.MatchingLabels{LABEL_UPGRADE: "true"},
			client.MatchingLabels{LABEL_MACHINESET: ms.Name},
		}...)
		if err != nil || len(machines.Items) != 1 {
			logger.Error(err, "failed to list extra upgrade machine")
			return false, err
		}

		machine := machines.Items[0]
		node := &corev1.Node{}
		err = c.Get(context.TODO(), types.NamespacedName{Name: machine.Status.NodeRef.Name}, node)
		if err != nil {
			logger.Error(err, "failed to get node")
			return false, err
		}
		nodeReady := false
		var nodeName string
		for _, con := range node.Status.Conditions {
			if con.Type == corev1.NodeReady && con.Status == corev1.ConditionTrue {
				nodeReady = true
				nodeName = node.Name
			}
		}
		if !nodeReady {
			allNodeReady = false
			if time.Now().After(startTime.Time.Add(timeOut)) {
				logger.Info("node is not ready within timeout time")
				return false, NewScaleTimeOutError(fmt.Sprintf("Timeout waiting for node:%s to become ready", nodeName))
			}
		}
	}
	if !allNodeReady {
		return false, nil
	}

	return allNodeReady, nil
}

// EnsureScaleDownNodes will remove extra MachineSets and report when the nodes are removed.
func (s *machineSetScaler) EnsureScaleDownNodes(c client.Client, nds drain.NodeDrainStrategy, logger logr.Logger) (bool, error) {
	upgradeMachinesets := &machineapi.MachineSetList{}

	err := c.List(context.TODO(), upgradeMachinesets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		return false, err
	}

	for _, ms := range upgradeMachinesets.Items {
		if ms.ObjectMeta.DeletionTimestamp == nil {
			err = c.Delete(context.TODO(), &ms)
			if err != nil {
				return false, err
			}
		}
	}

	if nds != nil {
		upgradeNodes, err := getExtraUpgradeNodes(c)
		if err != nil {
			return false, err
		}
		for _, n := range upgradeNodes.Items {
			res, err := nds.Execute(&n)
			for _, r := range res {
				logger.Info(r.Message)
			}
			if err != nil {
				return false, err
			}
		}
		for _, n := range upgradeNodes.Items {
			hasFailed, err := nds.HasFailed(&n)
			if err != nil {
				return false, err
			}
			if hasFailed {
				return false, NewDrainTimeOutError(n.Name)
			}
		}
	}

	// MachineSets control workers and infras nodes.
	originalMachineSets := &machineapi.MachineSetList{}
	err = c.List(context.TODO(), originalMachineSets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		NotMatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}

	// Desired replicas should match worker and infra count of nodes.
	nonMasterNodes := &corev1.NodeList{}
	err = c.List(context.TODO(), nonMasterNodes, []client.ListOption{
		NotMatchingLabels{machinery.MasterLabel: ""},
	}...)
	if err != nil {
		logger.Error(err, "failed to list nodes")
		return false, err
	}

	var desiredNodeCount int32 = 0
	for _, ms := range originalMachineSets.Items {
		desiredNodeCount += *ms.Spec.Replicas
	}

	if desiredNodeCount != int32(len(nonMasterNodes.Items)) {
		return false, nil
	}

	return true, nil
}

// NotMatchingLabels is a map of strings
type NotMatchingLabels map[string]string

// ApplyToList applies listOptions to NotMachingLabels
func (m NotMatchingLabels) ApplyToList(opts *client.ListOptions) {
	sel := NotSelectorFromSet(map[string]string(m))
	opts.LabelSelector = sel
}

// NotSelectorFromSet returns a labels.Selector
func NotSelectorFromSet(ls NotMatchingLabels) labels.Selector {
	if ls == nil || len(ls) == 0 {
		return labels.NewSelector()
	}
	selector := labels.Everything()
	for label, value := range ls {
		r, _ := labels.NewRequirement(label, selection.NotEquals, []string{value})
		selector = selector.Add(*r)
	}

	return selector
}

func getExtraUpgradeNodes(c client.Client) (*corev1.NodeList, error) {
	nodes := &corev1.NodeList{}
	err := c.List(context.TODO(), nodes)
	if err != nil {
		return nil, err
	}
	machines := &machineapi.MachineList{}
	err = c.List(context.TODO(), machines, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		return nil, err
	}

	extraUpgradeNodes := &corev1.NodeList{}
	for _, machine := range machines.Items {
		for _, node := range nodes.Items {
			if node.Name == machine.Status.NodeRef.Name {
				extraUpgradeNodes.Items = append(extraUpgradeNodes.Items, node)
			}
		}
	}

	return extraUpgradeNodes, nil
}

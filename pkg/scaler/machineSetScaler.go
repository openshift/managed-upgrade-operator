package scaler

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/go-logr/logr"
	machineapi "github.com/openshift/api/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/drain"
)

const (
	// LABEL_UPGRADE is the label used for managed upgrades
	LABEL_UPGRADE = "upgrade.managed.openshift.io"
	// LABEL_MACHINESET is the label used for machinesets
	LABEL_MACHINESET = "machine.openshift.io/cluster-api-machineset"
	// LABEL_MACHINE_POOL is the Hive machine pool label
	LABEL_MACHINE_POOL = "hive.openshift.io/machine-pool"
	// MACHINE_API_NAMESPACE is the namespace of the machine api
	MACHINE_API_NAMESPACE = "openshift-machine-api"
)

type machineSetScaler struct{}

// CanScale will check if the MachineSet scaler is capable of performing a scale-out event
func (s *machineSetScaler) CanScale(c client.Client, logger logr.Logger) (bool, error) {
	originalMachineSets := &machineapi.MachineSetList{}

	// Do we have an original "worker" machineset that can be scaled?
	err := c.List(context.TODO(), originalMachineSets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_MACHINE_POOL: "worker"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get original machinesets")
		return false, err
	}
	if len(originalMachineSets.Items) == 0 {
		// We require a worker machineset in order to perform a capacity scale
		return false, nil
	}

	// All our conditions are satisfied
	return true, nil
}

// EnsureScaleUpNodes will create a new MachineSet with 1 extra replicas for workers in every region and report when the nodes are ready.
// When extraMachinePools is non-empty, additional MachineSets matching those pool patterns are also scaled.
// Worker and extra pool fetching are independent: if workers are absent but extra pools match, only extra pools are scaled and vice versa.
func (s *machineSetScaler) EnsureScaleUpNodes(c client.Client, timeOut time.Duration, logger logr.Logger, extraMachinePools []string) (bool, error) {
	upgradeMachinesets := &machineapi.MachineSetList{}

	err := c.List(context.TODO(), upgradeMachinesets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get upgrade extra machinesets")
		return false, err
	}

	// Collect all MachineSets that need upgrade clones
	var allOriginals []machineapi.MachineSet

	// Fetch worker MachineSets
	workerMachineSets := &machineapi.MachineSetList{}
	err = c.List(context.TODO(), workerMachineSets, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_MACHINE_POOL: "worker"},
	}...)
	if err != nil {
		logger.Error(err, "failed to get worker machinesets")
	} else {
		allOriginals = append(allOriginals, workerMachineSets.Items...)
	}

	// Fetch extra machine pool MachineSets if configured
	if len(extraMachinePools) > 0 {
		extraSets, err := listMachineSetsByPoolPatterns(c, extraMachinePools, logger)
		if err != nil {
			logger.Error(err, "failed to list extra machine pool machinesets")
		} else if len(extraSets) > 0 {
			logger.Info(fmt.Sprintf("found %d extra machine pool machineset(s) to scale", len(extraSets)))
			allOriginals = append(allOriginals, extraSets...)
		}
	}

	if len(allOriginals) == 0 {
		logger.Info("no machinesets found to scale")
		return false, fmt.Errorf("failed to get any machinesets to scale")
	}

	originalMachineSets := &machineapi.MachineSetList{Items: allOriginals}

	created, err := extraMachineSetCreated(c, *originalMachineSets, *upgradeMachinesets, logger)
	if err != nil {
		return false, err
	}
	if created {
		// New machineset created, machines must not ready at the moment, so skip following steps
		logger.Info("created upgrade machinesets, will re-check their state on reconcile")
		return false, nil
	}

	allNodeReady, err := nodesAreReady(c, timeOut, *upgradeMachinesets, logger)
	if err != nil {
		return false, err
	}
	if !allNodeReady {
		logger.Info("not all nodes in the upgrade machinesets are ready yet")
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
		ms := ms
		if ms.DeletionTimestamp == nil {
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
		dsResult, err := handleDrainStrategy(c, nds, *upgradeNodes, logger)
		if err != nil {
			return dsResult, err
		}
	}

	//scaler block to verify upgrade machines scaled down.
	originalMachines := &machineapi.MachineList{}
	err = c.List(context.TODO(), originalMachines, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.MatchingLabels{LABEL_UPGRADE: "true"},
	}...)
	if err != nil {
		logger.Error(err, "Cannot get a list of extra upgrade machines")
		return false, err
	}

	//check if upgrade machine are present in the cluster.
	if len(originalMachines.Items) != 0 {
		for _, um := range originalMachines.Items {
			um := um
			logger.Info(fmt.Sprintf("Found upgrade machines to be terminated :%v", &um))
		}
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
	if len(ls) == 0 {
		return labels.NewSelector()
	}
	selector := labels.Everything()
	for label, value := range ls {
		r, _ := labels.NewRequirement(label, selection.NotEquals, []string{value})
		selector = selector.Add(*r)
	}

	return selector
}

// listMachineSetsByPoolPatterns returns MachineSets whose hive.openshift.io/machine-pool label value
// matches any of the given glob patterns. MachineSets with pool value "worker" are excluded because
// they are already handled separately.
func listMachineSetsByPoolPatterns(c client.Client, patterns []string, logger logr.Logger) ([]machineapi.MachineSet, error) {
	allPooled := &machineapi.MachineSetList{}
	err := c.List(context.TODO(), allPooled, []client.ListOption{
		client.InNamespace(MACHINE_API_NAMESPACE),
		client.HasLabels{LABEL_MACHINE_POOL},
	}...)
	if err != nil {
		return nil, fmt.Errorf("listing machinesets by pool label: %w", err)
	}

	var matched []machineapi.MachineSet
	for _, ms := range allPooled.Items {
		poolValue := ms.Labels[LABEL_MACHINE_POOL]
		if poolValue == "worker" {
			continue
		}
		for _, pattern := range patterns {
			ok, err := path.Match(pattern, poolValue)
			if err != nil {
				// Pattern was validated at config load time; this should not happen.
				return nil, fmt.Errorf("matching pattern %q against pool %q: %w", pattern, poolValue, err)
			}
			if ok {
				logger.Info(fmt.Sprintf("extra machine pool %q matched pattern %q (machineset %s)", poolValue, pattern, ms.Name))
				matched = append(matched, ms)
				break
			}
		}
	}
	return matched, nil
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
		if *machine.Status.Phase == "Running" || *machine.Status.Phase == "Deleting" {
			for _, node := range nodes.Items {
				if machine.Status.NodeRef == nil {
					return nil, fmt.Errorf("an upgrade machine %v exists but has no node association", machine.Name)
				}
				if node.Name == machine.Status.NodeRef.Name {
					extraUpgradeNodes.Items = append(extraUpgradeNodes.Items, node)
				}
			}
		}
	}

	return extraUpgradeNodes, nil
}

func extraMachineSetCreated(c client.Client, originalMachinesets, upgradeMachinesets machineapi.MachineSetList, logger logr.Logger) (bool, error) {
	for _, ms := range originalMachinesets.Items {

		found := false
		for _, ums := range upgradeMachinesets.Items {
			if ums.Name == ms.Name+"-upgrade" {
				found = true
			}
		}
		// extra machine already created
		if found {
			logger.Info(fmt.Sprintf("machineset for upgrade already created :%s", ms.Name))
			return false, nil
		}

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

		err := c.Create(context.TODO(), newMs)
		if err != nil {
			logger.Error(err, "failed to create machineset")
			return false, err
		}
	}

	return true, nil
}

func nodesAreReady(c client.Client, timeOut time.Duration, upgradeMachinesets machineapi.MachineSetList, logger logr.Logger) (bool, error) {

	for _, ms := range upgradeMachinesets.Items {
		//We assume the create time is the start time for scale up extra compute nodes
		startTime := ms.CreationTimestamp
		if ms.Status.Replicas != ms.Status.ReadyReplicas {

			if time.Now().After(startTime.Add(timeOut)) {
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
			if time.Now().After(startTime.Add(timeOut)) {
				logger.Info("node is not ready within timeout time")
				return false, NewScaleTimeOutError(fmt.Sprintf("Timeout waiting for node:%s to become ready", nodeName))
			}
			return false, nil
		}
	}
	return true, nil
}

func handleDrainStrategy(c client.Client, nds drain.NodeDrainStrategy, nodes corev1.NodeList, logger logr.Logger) (bool, error) {
	for _, n := range nodes.Items {
		n := n
		res, err := nds.Execute(&n, logger)
		for _, r := range res {
			logger.Info(r.Message)
		}
		if err != nil {
			return false, err
		}
	}
	for _, n := range nodes.Items {
		n := n
		hasFailed, err := nds.HasFailed(&n, logger)
		if err != nil {
			return false, err
		}
		if hasFailed {
			return false, NewDrainTimeOutError(n.Name)
		}
	}
	return true, nil
}

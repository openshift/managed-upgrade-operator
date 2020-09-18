package drain

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type ensurePodDeletionStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (fdps *ensurePodDeletionStrategy) Execute() (*DrainStrategyResult, error) {
	allPods := &corev1.PodList{}
	err := fdps.client.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	podsToDelete := pod.FilterPods(allPods, fdps.filters...)
	podsWithFinalizers := pod.FilterPods(podsToDelete, hasFinalizers)

	finRes, err := pod.RemoveFinalizersFromPod(fdps.client, podsWithFinalizers)
	if err != nil {
		return nil, err
	}

	delRes, err := pod.DeletePods(fdps.client, podsToDelete)
	if err != nil {
		return nil, err
	}

	result := &DrainStrategyResult{}
	result.HasExecuted = finRes.NumRemoved > 0 || delRes.NumMarkedForDeletion > 0
	if finRes.NumRemoved > 0 {
		result.Message = finRes.Message
	}
	if delRes.NumMarkedForDeletion > 0 {
		result.Message += " " + delRes.Message
	}

	return result, nil
}

func (fdps *ensurePodDeletionStrategy) HasFailed() (bool, error) {
	allPods := &corev1.PodList{}
	err := fdps.client.List(context.TODO(), allPods)
	if err != nil {
		return false, err
	}

	filterPods := pod.FilterPods(allPods, fdps.filters...)
	if len(filterPods.Items) == 0 {
		return false, nil
	}

	return true, nil
}

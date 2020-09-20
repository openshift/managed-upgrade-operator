package drain

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type podDeletionStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (pds *podDeletionStrategy) Execute() (*DrainStrategyResult, error) {
	allPods := &corev1.PodList{}
	err := pds.client.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	podsToDelete := pod.FilterPods(allPods, pds.filters...)

	delRes, err := pod.DeletePods(pds.client, podsToDelete)
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message:     delRes.Message,
		HasExecuted: delRes.NumMarkedForDeletion > 0,
	}, nil
}

func (fdps *podDeletionStrategy) HasFailed() (bool, error) {
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

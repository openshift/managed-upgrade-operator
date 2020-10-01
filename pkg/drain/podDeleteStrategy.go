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

func (pds *podDeletionStrategy) Execute(node *corev1.Node) (*DrainStrategyResult, error) {
	podsToDelete, err := pds.getPodList(node)
	if err != nil {
		return nil, err
	}

	gp := int64(0)
	res, err := pod.DeletePods(pds.client, podsToDelete, &client.DeleteOptions{GracePeriodSeconds: &gp})
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message:     res.Message,
		HasExecuted: res.NumMarkedForDeletion > 0,
	}, nil
}

func (pds *podDeletionStrategy) IsValid(node *corev1.Node) (bool, error) {
	targetPods, err := pds.getPodList(node)
	if err != nil {
		return false, err
	}

	return len(targetPods.Items) > 0, nil
}

func (pds *podDeletionStrategy) getPodList(node *corev1.Node) (*corev1.PodList, error) {
	allPods := &corev1.PodList{}
	err := pds.client.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	filters := append([]pod.PodPredicate{isOnNode(node)}, pds.filters...)
	return pod.FilterPods(allPods, filters...), nil
}

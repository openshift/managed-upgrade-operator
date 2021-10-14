package drain

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type podDeletionStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (pds *podDeletionStrategy) Execute(node *corev1.Node) (*DrainStrategyResult, error) {
	filters := append([]pod.PodPredicate{isOnNode(node)}, pds.filters...)
	podsToDelete, err := pod.GetPodList(pds.client, node, filters)
	if err != nil {
		return nil, err
	}

	gp := int64(0)
	res, err := pod.DeletePods(pds.client, podsToDelete, true, &client.DeleteOptions{GracePeriodSeconds: &gp})
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message:     res.Message,
		HasExecuted: res.NumMarkedForDeletion > 0,
	}, nil
}

func (pds *podDeletionStrategy) IsValid(node *corev1.Node) (bool, error) {
	filters := append([]pod.PodPredicate{isOnNode(node)}, pds.filters...)
	targetPods, err := pod.GetPodList(pds.client, node, filters)
	if err != nil {
		return false, err
	}

	return len(targetPods.Items) > 0, nil
}

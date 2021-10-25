package drain

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type removeFinalizersStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (rfs *removeFinalizersStrategy) Execute(node *corev1.Node) (*DrainStrategyResult, error) {
	filters := append([]pod.PodPredicate{isOnNode(node), hasFinalizers}, rfs.filters...)
	podsWithFinalizers, err := pod.GetPodList(rfs.client, node, filters)
	if err != nil {
		return nil, err
	}

	res, err := pod.RemoveFinalizersFromPod(rfs.client, podsWithFinalizers)
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message:     res.Message,
		HasExecuted: res.NumRemoved > 0,
	}, nil
}

func (rfs *removeFinalizersStrategy) IsValid(node *corev1.Node) (bool, error) {
	filters := append([]pod.PodPredicate{isOnNode(node), hasFinalizers}, rfs.filters...)
	targetPods, err := pod.GetPodList(rfs.client, node, filters)

	if err != nil {
		return false, err
	}

	return len(targetPods.Items) > 0, nil
}

package drain

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type removeFinalizersStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (rfs *removeFinalizersStrategy) Execute(node *corev1.Node) (*DrainStrategyResult, error) {
	podsWithFinalizers, err := rfs.getPodList(node)
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
	targetPods, err := rfs.getPodList(node)
	if err != nil {
		return false, err
	}

	return len(targetPods.Items) > 0, nil
}

func (rfs *removeFinalizersStrategy) getPodList(node *corev1.Node) (*corev1.PodList, error) {
	allPods := &corev1.PodList{}
	err := rfs.client.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	filters := append([]pod.PodPredicate{isOnNode(node), hasFinalizers}, rfs.filters...)
	return pod.FilterPods(allPods, filters...), nil
}

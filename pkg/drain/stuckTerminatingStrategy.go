package drain

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type stuckTerminatingStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (sts *stuckTerminatingStrategy) Execute(node *corev1.Node) (*DrainStrategyResult, error) {
	podsStuckTerminating, err := sts.getPodList(node)
	if err != nil {
		return nil, err
	}

	gp := int64(0)
	res, err := pod.DeletePods(sts.client, podsStuckTerminating, false, &client.DeleteOptions{GracePeriodSeconds: &gp})
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message:     res.Message,
		HasExecuted: res.NumMarkedForDeletion > 0,
	}, nil
}

func (sts *stuckTerminatingStrategy) IsValid(node *corev1.Node) (bool, error) {
	targetPods, err := sts.getPodList(node)
	if err != nil {
		return false, err
	}

	return len(targetPods.Items) > 0, nil
}

func (sts *stuckTerminatingStrategy) getPodList(node *corev1.Node) (*corev1.PodList, error) {
	allPods := &corev1.PodList{}
	err := sts.client.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	filters := append([]pod.PodPredicate{isOnNode(node), hasNoFinalizers, isTerminating}, sts.filters...)
	return pod.FilterPods(allPods, filters...), nil
}

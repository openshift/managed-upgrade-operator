package drain

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/managed-upgrade-operator/pkg/pod"
)

type stuckTerminatingStrategy struct {
	client  client.Client
	filters []pod.PodPredicate
}

func (sts *stuckTerminatingStrategy) Execute(node *corev1.Node, logger logr.Logger) (*DrainStrategyResult, error) {
	filters := append([]pod.PodPredicate{isOnNode(node), hasNoFinalizers, isTerminating}, sts.filters...)
	podsStuckTerminating, err := pod.GetPodList(sts.client, node, filters)

	if err != nil {
		return nil, err
	}

	gp := int64(0)
	res, err := pod.DeletePods(sts.client, logger, podsStuckTerminating, false, &client.DeleteOptions{GracePeriodSeconds: &gp})
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message:     res.Message,
		HasExecuted: res.NumMarkedForDeletion > 0,
	}, nil
}

func (sts *stuckTerminatingStrategy) IsValid(node *corev1.Node, logger logr.Logger) (bool, error) {
	filters := append([]pod.PodPredicate{isOnNode(node), hasNoFinalizers, isTerminating}, sts.filters...)
	targetPods, err := pod.GetPodList(sts.client, node, filters)
	if err != nil {
		return false, err
	}

	return len(targetPods.Items) > 0, nil
}

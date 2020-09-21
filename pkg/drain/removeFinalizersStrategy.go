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

func (rfs *removeFinalizersStrategy) Execute() (*DrainStrategyResult, error) {
	allPods := &corev1.PodList{}
	err := rfs.client.List(context.TODO(), allPods)
	if err != nil {
		return nil, err
	}

	podsWithFinalizers := pod.FilterPods(allPods, rfs.filters...)

	finRes, err := pod.RemoveFinalizersFromPod(rfs.client, podsWithFinalizers)
	if err != nil {
		return nil, err
	}

	return &DrainStrategyResult{
		Message: finRes.Message,
		HasExecuted: finRes.NumRemoved > 0,
	}, nil
}

func (fdps *removeFinalizersStrategy) HasFailed() (bool, error) {
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

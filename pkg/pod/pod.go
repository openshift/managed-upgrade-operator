package pod

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodPredicate func(corev1.Pod) bool

func FilterPods(podList *corev1.PodList, predicates ...PodPredicate) *corev1.PodList {
	filteredPods := &corev1.PodList{}
	for _, pod := range podList.Items {
		var match = true
		for _, p := range predicates {
			if !p(pod) {
				match = false
				break
			}
		}
		if match {
			filteredPods.Items = append(filteredPods.Items, pod)
		}
	}

	return filteredPods
}

type DeleteResult struct {
	Message              string
	NumMarkedForDeletion int
}

func DeletePods(c client.Client, pl *corev1.PodList) (*DeleteResult, error) {
	me := &multierror.Error{}
	var podsMarkedForDeletion []string
	for _, p := range pl.Items {
		if p.DeletionTimestamp == nil {
			err := c.Delete(context.TODO(), &p)
			if err != nil {
				me = multierror.Append(err, me)
			} else {
				podsMarkedForDeletion = append(podsMarkedForDeletion, p.Name)
			}
		}
	}

	return &DeleteResult{
		Message:              fmt.Sprintf("Pod(s) %s have been marked for deletion", strings.Join(podsMarkedForDeletion, ",")),
		NumMarkedForDeletion: len(podsMarkedForDeletion),
	}, me.ErrorOrNil()
}

type RemoveFinalizersResult struct {
	Message    string
	NumRemoved int
}

func RemoveFinalizersFromPod(c client.Client, pl *corev1.PodList) (*RemoveFinalizersResult, error) {
	var podsWithFinalizersRemoved []string
	me := &multierror.Error{}
	for _, p := range pl.Items {
		if len(p.ObjectMeta.GetFinalizers()) != 0 {
			emptyFinalizer := make([]string, 0)
			p.ObjectMeta.SetFinalizers(emptyFinalizer)

			err := c.Update(context.TODO(), &p)
			if err != nil {
				me = multierror.Append(err, me)
			} else {
				podsWithFinalizersRemoved = append(podsWithFinalizersRemoved, p.Name)
			}
		}
	}

	return &RemoveFinalizersResult{
		Message:    fmt.Sprintf("Finalizers removed for pods: %s", strings.Join(podsWithFinalizersRemoved, ",")),
		NumRemoved: len(podsWithFinalizersRemoved),
	}, me.ErrorOrNil()
}

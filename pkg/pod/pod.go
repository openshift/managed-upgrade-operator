package pod

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodPredicate is a predicate function for a given Pod
type PodPredicate func(corev1.Pod) bool

// FilterPods filters a podList and returns a PodList matching the predicates
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

// DeleteResult holds fields describing the result of a pod deletion
type DeleteResult struct {
	Message              string
	NumMarkedForDeletion int
}

// DeletePods attempts to delete a given PodList and returns a DeleteResult and error
func DeletePods(c client.Client, pl *corev1.PodList, ignoreAlreadyDeleting bool, options ...client.DeleteOption) (*DeleteResult, error) {
	me := &multierror.Error{}
	var podsMarkedForDeletion []string
	for _, p := range pl.Items {
		if !ignoreAlreadyDeleting || p.DeletionTimestamp == nil {
			err := c.Delete(context.TODO(), &p, options...)
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

// RemoveFinalizersResult is a type that describes the result of removing a finalizer
type RemoveFinalizersResult struct {
	Message    string
	NumRemoved int
}

// RemoveFinalizersFromPod attempts to remove the finalizers from a given PodList and returns a RemoveFinalizersResult and error
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

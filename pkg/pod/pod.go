package pod

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
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
func DeletePods(c client.Client, logger logr.Logger, pl *corev1.PodList, ignoreAlreadyDeleting bool, options ...client.DeleteOption) (*DeleteResult, error) {
	me := &multierror.Error{}
	var podsMarkedForDeletion []string
	for _, p := range pl.Items {
		p := p
		if !ignoreAlreadyDeleting || p.DeletionTimestamp == nil {
			logger.Info(fmt.Sprintf("Applying pod deletion drain strategy to pod %v/%v", p.Namespace, p.Name))
			err := c.Delete(context.TODO(), &p, options...)
			if err != nil {
				logger.Error(err, fmt.Sprintf("failed to delete the pod %v/%v", p.Namespace, p.Name))
				me = multierror.Append(err, me)
			} else {
				podsMarkedForDeletion = append(podsMarkedForDeletion, p.Name)
			}
		} else {
			logger.Info(fmt.Sprintf("Ignoring deleting pod %v because it is already being deleted", p.Name))
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
func RemoveFinalizersFromPod(c client.Client, logger logr.Logger, pl *corev1.PodList) (*RemoveFinalizersResult, error) {
	var podsWithFinalizersRemoved []string
	me := &multierror.Error{}
	for _, p := range pl.Items {
		p := p
		if len(p.GetFinalizers()) != 0 {
			logger.Info(fmt.Sprintf("Applying remove finalizer strategy to pod %v/%v", p.Namespace, p.Name))
			emptyFinalizer := make([]string, 0)
			p.SetFinalizers(emptyFinalizer)

			err := c.Update(context.TODO(), &p)
			if err != nil {
				logger.Error(err, fmt.Sprintf("failed to remove finalizer from the pod %v/%v", p.Namespace, p.Name))
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

// Get pod list on a given node matching pod predicate
func GetPodList(c client.Client, n *corev1.Node, filters []PodPredicate) (*corev1.PodList, error) {

	fieldSelector, err := fields.ParseSelector("spec.nodeName=" + n.Name)
	if err != nil {
		return nil, err
	}

	allPods := &corev1.PodList{}
	err = c.List(context.TODO(), allPods, &client.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, err
	}

	return FilterPods(allPods, filters...), nil
}

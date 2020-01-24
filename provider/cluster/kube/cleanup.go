package kube

import (
	"github.com/ovrclk/akash/manifest"
	mtypes "github.com/ovrclk/akash/x/market/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
)

func cleanupStaleResources(kc kubernetes.Interface, lid mtypes.LeaseID, group *manifest.Group) error {
	ns := lidNS(lid)

	// build label selector for objects not in current manifest group
	svcnames := make([]string, 0, len(group.Services))
	for _, svc := range group.Services {
		svcnames = append(svcnames, svc.Name)
	}

	req1, err := labels.NewRequirement(akashManifestServiceLabelName, selection.NotIn, svcnames)
	if err != nil {
		return err
	}
	req2, err := labels.NewRequirement(akashManagedLabelName, selection.Equals, []string{"true"})
	if err != nil {
		return err
	}
	selector := labels.NewSelector().Add(*req1).Add(*req2).String()

	// delete stale deployments
	if err := kc.AppsV1().Deployments(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return err
	}

	// delete stale ingresses
	if err := kc.ExtensionsV1beta1().Ingresses(ns).DeleteCollection(&metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return err
	}

	// delete stale services (no DeleteCollection)
	services, err := kc.CoreV1().Services(ns).List(metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return err
	}
	for _, svc := range services.Items {
		if err := kc.CoreV1().Services(ns).Delete(svc.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil

}

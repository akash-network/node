package kube

import (
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func applyNS(kc kubernetes.Interface, b *nsBuilder) error {
	obj, err := kc.CoreV1().Namespaces().Get(b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Update(obj)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Create(obj)
		}
	}
	return err
}

func applyDeployment(kc kubernetes.Interface, b *deploymentBuilder) error {
	obj, err := kc.AppsV1().Deployments(b.ns()).Get(b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.AppsV1().Deployments(b.ns()).Update(obj)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.AppsV1().Deployments(b.ns()).Create(obj)
		}
	}
	return err
}

func applyService(kc kubernetes.Interface, b *serviceBuilder) error {
	obj, err := kc.CoreV1().Services(b.ns()).Get(b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.CoreV1().Services(b.ns()).Update(obj)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.CoreV1().Services(b.ns()).Create(obj)
		}
	}
	return err
}

func applyIngress(kc kubernetes.Interface, b *ingressBuilder) error {
	obj, err := kc.ExtensionsV1beta1().Ingresses(b.ns()).Get(b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.ExtensionsV1beta1().Ingresses(b.ns()).Update(obj)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.ExtensionsV1beta1().Ingresses(b.ns()).Create(obj)
		}
	}
	return err
}

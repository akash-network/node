package kube

import (
	"context"

	akashv1 "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func applyNS(ctx context.Context, kc kubernetes.Interface, b *nsBuilder) error {
	obj, err := kc.CoreV1().Namespaces().Get(ctx, b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Update(ctx, obj, metav1.UpdateOptions{})
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Create(ctx, obj, metav1.CreateOptions{})
		}
	}
	return err
}

// Apply list of Network Policies
func applyNetPolicies(ctx context.Context, kc kubernetes.Interface, b *netPolBuilder) error {
	var err error
	var obj *netv1.NetworkPolicy

	policies, err := b.create()
	fn := func(np *netv1.NetworkPolicy) {
		obj, err = kc.NetworkingV1().NetworkPolicies(b.ns()).Get(ctx, np.Name, metav1.GetOptions{})
		switch {
		case err == nil:
			obj, err = b.update(obj)
			if err == nil {
				_, err = kc.NetworkingV1().NetworkPolicies(b.ns()).Update(ctx, np, metav1.UpdateOptions{})
			}
		case errors.IsNotFound(err):
			_, err = kc.NetworkingV1().NetworkPolicies(b.ns()).Create(ctx, np, metav1.CreateOptions{})
		}
	}

	for _, pol := range policies {
		if err != nil {
			break // short circuit running due to error returned
		}
		fn(pol)
	}

	return err
}

func applyDeployment(ctx context.Context, kc kubernetes.Interface, b *deploymentBuilder) error {
	obj, err := kc.AppsV1().Deployments(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.AppsV1().Deployments(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.AppsV1().Deployments(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
		}
	}
	return err
}

func applyService(ctx context.Context, kc kubernetes.Interface, b *serviceBuilder) error {
	obj, err := kc.CoreV1().Services(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.CoreV1().Services(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.CoreV1().Services(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
		}
	}
	return err
}

func applyIngress(ctx context.Context, kc kubernetes.Interface, b *ingressBuilder) error {
	obj, err := kc.ExtensionsV1beta1().Ingresses(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.ExtensionsV1beta1().Ingresses(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.ExtensionsV1beta1().Ingresses(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
		}
	}
	return err
}

func prepareEnvironment(ctx context.Context, kc kubernetes.Interface, ns string) error {
	_, err := kc.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		obj := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
				Labels: map[string]string{
					akashManagedLabelName: "true",
				},
			},
		}
		_, err = kc.CoreV1().Namespaces().Create(ctx, obj, metav1.CreateOptions{})
	}
	return err
}

func applyManifest(ctx context.Context, kc akashv1.Interface, b *manifestBuilder) error {
	obj, err := kc.AkashV1().Manifests(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})
	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.AkashV1().Manifests(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.AkashV1().Manifests(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
		}
	}
	return err
}

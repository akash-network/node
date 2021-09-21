package kube

// nolint:deadcode,golint

import (
	"context"

	akashv1 "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func applyNS(ctx context.Context, kc kubernetes.Interface, b *nsBuilder) error {
	obj, err := kc.CoreV1().Namespaces().Get(ctx, b.name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "namespaces-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "namespaces-update", err)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "namespaces-create", err)
		}
	}
	return err
}

// Apply list of Network Policies
func applyNetPolicies(ctx context.Context, kc kubernetes.Interface, b *netPolBuilder) error {
	var err error

	policies, err := b.create()
	if err != nil {
		return err
	}

	for _, pol := range policies {
		obj, err := kc.NetworkingV1().NetworkPolicies(b.ns()).Get(ctx, pol.Name, metav1.GetOptions{})
		metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "networking-policies-get", err, errors.IsNotFound)

		switch {
		case err == nil:
			_, err = b.update(obj)
			if err == nil {
				_, err = kc.NetworkingV1().NetworkPolicies(b.ns()).Update(ctx, pol, metav1.UpdateOptions{})
				metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "networking-policies-update", err)
			}
		case errors.IsNotFound(err):
			_, err = kc.NetworkingV1().NetworkPolicies(b.ns()).Create(ctx, pol, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "networking-policies-create", err)
		}
		if err != nil {
			break
		}
	}

	return err
}

// TODO: re-enable.  see #946
// func applyRestrictivePodSecPoliciesToNS(ctx context.Context, kc kubernetes.Interface, p *pspRestrictedBuilder) error {
// 	obj, err := kc.PolicyV1beta1().PodSecurityPolicies().Get(ctx, p.name(), metav1.GetOptions{})
// 	switch {
// 	case err == nil:
// 		obj, err = p.update(obj)
// 		if err == nil {
// 			_, err = kc.PolicyV1beta1().PodSecurityPolicies().Update(ctx, obj, metav1.UpdateOptions{})
// 		}
// 	case errors.IsNotFound(err):
// 		obj, err = p.create()
// 		if err == nil {
// 			_, err = kc.PolicyV1beta1().PodSecurityPolicies().Create(ctx, obj, metav1.CreateOptions{})
// 		}
// 	}
// 	return err
// }

func applyDeployment(ctx context.Context, kc kubernetes.Interface, b *deploymentBuilder) error {
	obj, err := kc.AppsV1().Deployments(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "deployments-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.update(obj)

		if err == nil {
			_, err = kc.AppsV1().Deployments(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "deployments-update", err)

		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.AppsV1().Deployments(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "deployments-create", err)
		}
	}
	return err
}

func applyService(ctx context.Context, kc kubernetes.Interface, b *serviceBuilder) error {
	obj, err := kc.CoreV1().Services(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "services-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.CoreV1().Services(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "services-update", err)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.CoreV1().Services(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "services-create", err)
		}
	}
	return err
}

func prepareEnvironment(ctx context.Context, kc kubernetes.Interface, ns string) error {
	_, err := kc.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "namespaces-get", err, errors.IsNotFound)

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
		metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "namespaces-create", err)
	}
	return err
}

func applyManifest(ctx context.Context, kc akashv1.Interface, b *manifestBuilder) error {
	obj, err := kc.AkashV1().Manifests(b.ns()).Get(ctx, b.name(), metav1.GetOptions{})

	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "akash-manifests-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.update(obj)
		if err == nil {
			_, err = kc.AkashV1().Manifests(b.ns()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "akash-manifests-update", err)
		}
	case errors.IsNotFound(err):
		obj, err = b.create()
		if err == nil {
			_, err = kc.AkashV1().Manifests(b.ns()).Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "akash-manifests-create", err)
		}
	}
	return err
}

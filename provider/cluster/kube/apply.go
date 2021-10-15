package kube

// nolint:deadcode,golint

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	akashv1 "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	metricsutils "github.com/ovrclk/akash/util/metrics"
)

func applyNS(ctx context.Context, kc kubernetes.Interface, b builder.NS) error {
	obj, err := kc.CoreV1().Namespaces().Get(ctx, b.Name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "namespaces-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.Update(obj)
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "namespaces-update", err)
		}
	case errors.IsNotFound(err):
		obj, err = b.Create()
		if err == nil {
			_, err = kc.CoreV1().Namespaces().Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "namespaces-create", err)
		}
	}
	return err
}

// Apply list of Network Policies
func applyNetPolicies(ctx context.Context, kc kubernetes.Interface, b builder.NetPol) error {
	var err error

	policies, err := b.Create()
	if err != nil {
		return err
	}

	for _, pol := range policies {
		obj, err := kc.NetworkingV1().NetworkPolicies(b.NS()).Get(ctx, pol.Name, metav1.GetOptions{})
		metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "networking-policies-get", err, errors.IsNotFound)

		switch {
		case err == nil:
			_, err = b.Update(obj)
			if err == nil {
				_, err = kc.NetworkingV1().NetworkPolicies(b.NS()).Update(ctx, pol, metav1.UpdateOptions{})
				metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "networking-policies-update", err)
			}
		case errors.IsNotFound(err):
			_, err = kc.NetworkingV1().NetworkPolicies(b.NS()).Create(ctx, pol, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "networking-policies-create", err)
		}
		if err != nil {
			break
		}
	}

	return err
}

// TODO: re-enable.  see #946
// func applyRestrictivePodSecPoliciesToNS(ctx context.Context, kc kubernetes.Interface, p builder.PspRestricted) error {
// 	obj, err := kc.PolicyV1beta1().PodSecurityPolicies().Get(ctx, p.Name(), metav1.GetOptions{})
// 	switch {
// 	case err == nil:
// 		obj, err = p.Update(obj)
// 		if err == nil {
// 			_, err = kc.PolicyV1beta1().PodSecurityPolicies().Update(ctx, obj, metav1.UpdateOptions{})
// 		}
// 	case errors.IsNotFound(err):
// 		obj, err = p.Create()
// 		if err == nil {
// 			_, err = kc.PolicyV1beta1().PodSecurityPolicies().Create(ctx, obj, metav1.CreateOptions{})
// 		}
// 	}
// 	return err
// }

func applyDeployment(ctx context.Context, kc kubernetes.Interface, b builder.Deployment) error {
	obj, err := kc.AppsV1().Deployments(b.NS()).Get(ctx, b.Name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "deployments-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.Update(obj)

		if err == nil {
			_, err = kc.AppsV1().Deployments(b.NS()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "deployments-update", err)

		}
	case errors.IsNotFound(err):
		obj, err = b.Create()
		if err == nil {
			_, err = kc.AppsV1().Deployments(b.NS()).Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "deployments-create", err)
		}
	}
	return err
}

func applyStatefulSet(ctx context.Context, kc kubernetes.Interface, b builder.StatefulSet) error {
	obj, err := kc.AppsV1().StatefulSets(b.NS()).Get(ctx, b.Name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "deployments-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.Update(obj)

		if err == nil {
			_, err = kc.AppsV1().StatefulSets(b.NS()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "deployments-update", err)

		}
	case errors.IsNotFound(err):
		obj, err = b.Create()
		if err == nil {
			_, err = kc.AppsV1().StatefulSets(b.NS()).Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "deployments-create", err)
		}
	}
	return err
}

func applyService(ctx context.Context, kc kubernetes.Interface, b builder.Service) error {
	obj, err := kc.CoreV1().Services(b.NS()).Get(ctx, b.Name(), metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "services-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.Update(obj)
		if err == nil {
			_, err = kc.CoreV1().Services(b.NS()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "services-update", err)
		}
	case errors.IsNotFound(err):
		obj, err = b.Create()
		if err == nil {
			_, err = kc.CoreV1().Services(b.NS()).Create(ctx, obj, metav1.CreateOptions{})
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
					builder.AkashManagedLabelName: "true",
				},
			},
		}
		_, err = kc.CoreV1().Namespaces().Create(ctx, obj, metav1.CreateOptions{})
		metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "namespaces-create", err)
	}
	return err
}

func applyManifest(ctx context.Context, kc akashv1.Interface, b builder.Manifest) error {
	obj, err := kc.AkashV1().Manifests(b.NS()).Get(ctx, b.Name(), metav1.GetOptions{})

	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "akash-manifests-get", err, errors.IsNotFound)

	switch {
	case err == nil:
		obj, err = b.Update(obj)
		if err == nil {
			_, err = kc.AkashV1().Manifests(b.NS()).Update(ctx, obj, metav1.UpdateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "akash-manifests-update", err)
		}
	case errors.IsNotFound(err):
		obj, err = b.Create()
		if err == nil {
			_, err = kc.AkashV1().Manifests(b.NS()).Create(ctx, obj, metav1.CreateOptions{})
			metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "akash-manifests-create", err)
		}
	}
	return err
}

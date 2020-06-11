package kube

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// settings configures k8s object generation such that it is customized to the
// cluster environment that is being used.
// For instance, GCP requires a different service type than minikube.
type settings struct {
	// gcp:    NodePort
	// others: ClusterIP
	DeploymentServiceType corev1.ServiceType `env:"AKASH_DEPLOYMENT_SERVICE_TYPE" envDefault:"ClusterIP"`

	// gcp:    False
	// others: true
	DeploymentIngressStaticHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS" envDefault:"true"`
	// Ingress domain to map deployments to
	DeploymentIngressDomain string `env:"AKASH_DEPLOYMENT_INGRESS_DOMAIN"`

	DeploymentIngressExposeLBHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_EXPOSE_LB_HOSTS" envDefault:"true"`
}

var errSettingsValidation = errors.New("settings validation")

func validateSettings(settings settings) error {
	if settings.DeploymentIngressStaticHosts && settings.DeploymentIngressDomain == "" {
		return errors.Wrap(errSettingsValidation, "empty ingress domain")
	}
	return nil
}

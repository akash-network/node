package kube

import (
	"github.com/caarlos0/env"
	corev1 "k8s.io/api/core/v1"
)

// Config is the struct that stores kube config
type Config struct {
	// gcp:    NodePort
	// others: ClusterIP
	DeploymentServiceType corev1.ServiceType `env:"AKASH_DEPLOYMENT_SERVICE_TYPE" envDefault:"NodePort"`

	// gcp:    False
	// others: true
	DeploymentIngressStaticHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS" envDefault:"true"`
	// Ingress domain to map deployments to
	DeploymentIngressDomain string `env:"AKASH_DEPLOYMENT_INGRESS_DOMAIN"`

	DeploymentIngressExposeLBHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_EXPOSE_LB_HOSTS" envDefault:"true"`
}

var config = Config{}

func init() {
	if err := env.Parse(&config); err != nil {
		panic(err)
	}
}

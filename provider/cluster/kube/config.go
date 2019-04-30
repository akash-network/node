package kube

import (
	"github.com/caarlos0/env"
	corev1 "k8s.io/api/core/v1"
)

type config_ struct {

	// gcp:    NodePort
	// others: ClusterIP
	DeploymentServiceType corev1.ServiceType `env:"AKASH_DEPLOYMENT_SERVICE_TYPE" envDefault:"NodePort"`

	// gcp:    False
	// others: true
	DeploymentIngressStaticHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_STATIC_HOSTS" envDefault:"false"`

	DeploymentIngressExposeLBHosts bool `env:"AKASH_DEPLOYMENT_INGRESS_EXPOSE_LB_HOSTS" envDefault:"true"`
}

var config = config_{}

func init() {
	if err := env.Parse(&config); err != nil {
		panic(err)
	}
}

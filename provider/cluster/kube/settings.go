package kube

import (
	"fmt"
	validation_util "github.com/ovrclk/akash/util/validation"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// settings configures k8s object generation such that it is customized to the
// cluster environment that is being used.
// For instance, GCP requires a different service type than minikube.
type Settings struct {
	// gcp:    NodePort
	// others: ClusterIP
	DeploymentServiceType corev1.ServiceType

	// gcp:    false
	// others: true
	DeploymentIngressStaticHosts bool
	// Ingress domain to map deployments to
	DeploymentIngressDomain string

	// Return load balancer host in lease status command ?
	// gcp:    true
	// others: optional
	DeploymentIngressExposeLBHosts bool

	// Global hostname for arbitrary ports
	ClusterPublicHostname string

	// NetworkPoliciesEnabled determines if NetworkPolicies should be installed.
	NetworkPoliciesEnabled bool

	CPUCommitLevel     float64
	MemoryCommitLevel  float64
	StorageCommitLevel float64

	// path to kube config file, may be left blank
	ConfigPath             string
	DeploymentRuntimeClass string
}

var errSettingsValidation = errors.New("settings validation")

func validateSettings(settings Settings) error {
	if settings.DeploymentIngressStaticHosts {
		if settings.DeploymentIngressDomain == "" {
			return errors.Wrap(errSettingsValidation, "empty ingress domain")
		}

		if !validation_util.IsDomainName(settings.DeploymentIngressDomain) {
			return fmt.Errorf("%w: invalid domain name %q", errSettingsValidation, settings.DeploymentIngressDomain)
		}
	}

	return nil
}

func NewDefaultSettings() Settings {
	return Settings{
		DeploymentServiceType:          corev1.ServiceTypeClusterIP,
		DeploymentIngressStaticHosts:   false,
		DeploymentIngressExposeLBHosts: false,
		NetworkPoliciesEnabled:         false,
	}
}

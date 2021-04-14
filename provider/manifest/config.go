package manifest

import "time"

type ServiceConfig struct {
	HTTPServicesRequireAtLeastOneHost bool
	ManifestTimeout                   time.Duration
}

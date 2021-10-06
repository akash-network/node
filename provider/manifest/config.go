package manifest

import "time"

type ServiceConfig struct {
	HTTPServicesRequireAtLeastOneHost bool
	ManifestTimeout                   time.Duration
	RPCQueryTimeout                   time.Duration
	CachedResultMaxAge                time.Duration
}

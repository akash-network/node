package manifest

import "time"

type config struct {
	ManifestLingerDuration time.Duration `env:"AKASH_MANIFEST_LINGER_DURATION" envDefault:"5m"`
}

package provider

import "time"

type config struct {
	ClusterWaitReadyDuration time.Duration `env:"AKASH_CLUSTER_WAIT_READY_DURATION" envDefault:"5s"`
}

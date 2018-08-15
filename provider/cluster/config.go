package cluster

import "time"

type config struct {
	InventoryResourcePollPeriod     time.Duration `env:"AKASH_INVENTORY_RESOURCE_POLL_PERIOD" envDefault:"5s"`
	InventoryResourceDebugFrequency uint          `env:"AKASH_INVENTORY_RESOURCE_DEBUG_FREQUENCY" envDefault:"10"`
}

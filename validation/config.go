package validation

import (
	"github.com/caarlos0/env"
)

type config struct {
	MaxUnitCPU    uint `env:"AKASH_MAX_UNIT_CPU"    envDefault:"10"`
	MaxUnitMemory uint `env:"AKASH_MAX_UNIT_MEMORY" envDefault:"1073741824"` // 1Gi
	MaxUnitDisk   uint `env:"AKASH_MAX_UNIT_DISK"   envDefault:"1073741824"` // 1Gi
	MaxUnitCount  uint `env:"AKASH_MAX_UNIT_COUNT"  envDefault:"10"`
	MaxUnitPrice  uint `env:"AKASH_MAX_UNIT_PRICE"  envDefault:"10000"`

	MaxGroupCount uint `env:"AKASH_MAX_GROUP_COUNT" envDefault:"10"`
}

var defaultConfig = config{}

func init() {
	if err := env.Parse(&defaultConfig); err != nil {
		panic(err)
	}
}

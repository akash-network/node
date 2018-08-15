package validation

import (
	"github.com/caarlos0/env"
)

type config struct {
	MaxUnitCPU    uint `env:"AKASH_MAX_UNIT_CPU"    envDefault:"500"`
	MaxUnitMemory uint `env:"AKASH_MAX_UNIT_MEMORY" envDefault:"1073741824"` // 1Gi
	MaxUnitDisk   uint `env:"AKASH_MAX_UNIT_DISK"   envDefault:"1073741824"` // 1Gi
	MaxUnitCount  uint `env:"AKASH_MAX_UNIT_COUNT"  envDefault:"10"`
	MaxUnitPrice  uint `env:"AKASH_MAX_UNIT_PRICE"  envDefault:"10000"`

	MinUnitCPU    uint `env:"AKASH_MIN_UNIT_CPU"    envDefault:"10"`
	MinUnitMemory uint `env:"AKASH_MIN_UNIT_MEMORY" envDefault:"1024"`
	MinUnitDisk   uint `env:"AKASH_MIN_UNIT_DISK"   envDefault:"1024"`
	MinUnitCount  uint `env:"AKASH_MIN_UNIT_COUNT"  envDefault:"1"`
	MinUnitPrice  uint `env:"AKASH_MIN_UNIT_PRICE"  envDefault:"1"`

	MaxGroupCount int `env:"AKASH_MAX_GROUP_COUNT" envDefault:"10"`
	MaxGroupUnits int `env:"AKASH_MAX_GROUP_UNITS" envDefault:"10"`

	MaxGroupCPU    int64 `env:"AKASH_MAX_GROUP_CPU"    envDefault:"1000"`
	MaxGroupMemory int64 `env:"AKASH_MAX_GROUP_MEMORY" envDefault:"1073741824"` // 1Gi
	MaxGroupDisk   int64 `env:"AKASH_MAX_GROUP_DISK"   envDefault:"5368709120"` // 5Gi

	MinGroupMemPrice int64 `env:"AKASH_MEM_PRICE_MIN" envDefault:"50"`
	MaxGroupMemPrice int64 `env:"AKASH_MEM_PRICE_MAX" envDefault:"150"`
}

var defaultConfig = config{}

func init() {
	if err := env.Parse(&defaultConfig); err != nil {
		panic(err)
	}
}

func Config() config {
	return defaultConfig
}

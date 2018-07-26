package bidengine

type config struct {
	FulfillmentCPUMax    int64 `env:"AKASH_FULFILLMENT_CPU_MAX" envDefault:"1000"`
	FulfillmentMemoryMax int64 `env:"AKASH_FULFILLMENT_MEMORY_MAX" envDefault:"1073741824"` // 1Gi
	FulfillmentDiskMax   int64 `env:"AKASH_FULFILLMENT_DISK_MAX" envDefault:"5368709120"`   // 5Gi

	FulfillmentMemPriceMin int64 `env:"AKASH_MEM_PRICE_MIN" envDefault:"50"`
	FulfillmentMemPriceMax int64 `env:"AKASH_MEM_PRICE_MAX" envDefault:"150"`
}

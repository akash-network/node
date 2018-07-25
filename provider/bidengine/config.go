package bidengine

type config struct {
	FulfillmentCPUMax uint `env:"AKASH_PROVIDER_MAX_FULFILLMENT_CPU" envDefault:"1000"`

	FulfillmentMemPriceMin uint `env:"AKASH_PROVIDER_MEM_PRICE_MIN" envDefault:"50"`
	FulfillmentMemPriceMax uint `env:"AKASH_PROVIDER_MEM_PRICE_MAX" envDefault:"150"`
}

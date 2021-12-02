package bidengine

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/ovrclk/akash/types/v1beta2"
)

type Config struct {
	PricingStrategy BidPricingStrategy
	Deposit         sdk.Coin
	BidTimeout      time.Duration
	Attributes      types.Attributes
	MaxGroupVolumes int
}

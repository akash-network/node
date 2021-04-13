package bidengine

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"time"
)

type Config struct {
	PricingStrategy BidPricingStrategy
	Deposit         sdk.Coin
	BidTimeout      time.Duration
}

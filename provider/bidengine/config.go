package bidengine

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/types"
)

type Config struct {
	PricingStrategy BidPricingStrategy
	Deposit         sdk.Coin
	BidTimeout      time.Duration
	Attributes      types.Attributes
}

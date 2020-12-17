package bidengine

import sdk "github.com/cosmos/cosmos-sdk/types"

type Config struct {
	PricingStrategy BidPricingStrategy
	Deposit         sdk.Coin
}

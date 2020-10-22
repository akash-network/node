package query

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// CoinDetails represents bonded and unbonded coin details
type CoinDetails struct {
	Bonded   sdk.Coins `json:"bonded"`
	Unbonded sdk.Coins `json:"unbonded"`
}

// Supply represents total coins vested, available and circulating supply
type Supply struct {
	Vesting     CoinDetails `json:"vesting"`
	Available   CoinDetails `json:"available"`
	Circulating sdk.Coins   `json:"circulating"`
	Total       sdk.Coins   `json:"total"`
}

// String method of Supply
func (Supply) String() string {
	return ""
}

package market

import (
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
)

// StoreKey defines storekey of market module
const (
	StoreKey   = types.StoreKey
	ModuleName = types.ModuleName
)

// Keeper defines keeper of market module
type (
	Keeper = keeper.Keeper
)

// NewKeeper creates new keeper instance of market module
var (
	NewKeeper = keeper.NewKeeper
)

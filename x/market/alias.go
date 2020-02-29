package market

import (
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
)

const (
	// StoreKey represents storekey of market module
	StoreKey = types.StoreKey
	// ModuleName represents current module name
	ModuleName = types.ModuleName
)

type (
	// Keeper defines keeper of market module
	Keeper = keeper.Keeper
)

var (
	// NewKeeper creates new keeper instance of market module
	NewKeeper = keeper.NewKeeper
)

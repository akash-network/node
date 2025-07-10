package market

import (
	v1 "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/node/x/market/keeper"
)

const (
	// StoreKey represents storekey of market module
	StoreKey = v1.StoreKey
	// ModuleName represents current module name
	ModuleName = v1.ModuleName
)

type (
	// Keeper defines keeper of market module
	Keeper = keeper.Keeper
)

var (
	// NewKeeper creates new keeper instance of market module
	NewKeeper = keeper.NewKeeper
)

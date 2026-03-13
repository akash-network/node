package market

import (
	mv1 "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/node/v2/x/market/keeper"
)

const (
	// StoreKey represents storekey of market module
	StoreKey = mv1.StoreKey
	// ModuleName represents current module name
	ModuleName = mv1.ModuleName
)

type (
	// Keeper defines keeper of market module
	Keeper = keeper.Keeper
)

var (
	// NewKeeper creates new keeper instance of market module
	NewKeeper = keeper.NewKeeper
)

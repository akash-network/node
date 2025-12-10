package market

import (
	mtypes "pkg.akt.dev/go/node/market/v1"

	"pkg.akt.dev/node/v2/x/market/keeper"
)

const (
	// StoreKey represents storekey of market module
	StoreKey = mtypes.StoreKey
	// ModuleName represents current module name
	ModuleName = mtypes.ModuleName
)

type (
	// Keeper defines keeper of market module
	Keeper = keeper.Keeper
)

var (
	// NewKeeper creates new keeper instance of market module
	NewKeeper = keeper.NewKeeper
)

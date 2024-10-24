package inflation

import (
	types "pkg.akt.dev/go/node/inflation/v1beta3"

	"pkg.akt.dev/node/x/inflation/keeper"
)

const (
	// StoreKey represents storekey of inflation module
	StoreKey = types.StoreKey
	// ModuleName represents current module name
	ModuleName = types.ModuleName
)

type (
	// Keeper defines keeper of inflation module
	Keeper = keeper.IKeeper
)

var (
	// NewKeeper creates new keeper instance of inflation module
	NewKeeper = keeper.NewKeeper
)

package inflation

import (
	types "github.com/akash-network/akash-api/go/node/inflation/v1beta3"

	"github.com/akash-network/node/x/inflation/keeper"
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

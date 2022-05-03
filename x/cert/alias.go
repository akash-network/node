package cert

import (
	"github.com/ovrclk/akash/x/cert/keeper"
	types "github.com/ovrclk/akash/x/cert/types/v1beta2"
)

const (
	// StoreKey represents storekey of provider module
	StoreKey = types.StoreKey
	// ModuleName represents current module name
	ModuleName = types.ModuleName
)

type (
	// Keeper defines keeper of provider module
	Keeper = keeper.Keeper
)

// NewKeeper creates new keeper instance of provider module
var NewKeeper = keeper.NewKeeper

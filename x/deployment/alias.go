package deployment

import (
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

// StoreKey defines storekey of deployment module
const (
	StoreKey   = types.StoreKey
	ModuleName = types.ModuleName
)

// Keeper defines keeper of deployment module
type (
	Keeper = keeper.Keeper
)

// NewKeeper creates new keeper instance of deployment module
var (
	NewKeeper = keeper.NewKeeper
)

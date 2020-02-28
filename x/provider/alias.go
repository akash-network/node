package provider

import (
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
)

// StoreKey defines storekey of provider module
const (
	StoreKey   = types.StoreKey
	ModuleName = types.ModuleName
)

// Keeper defines keeper of provider module
type Keeper keeper.Keeper

// NewKeeper creates new keeper instance of provider module
var (
	NewKeeper = keeper.NewKeeper
)

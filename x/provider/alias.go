package provider

import (
	"github.com/akash-network/node/x/provider/keeper"
	types "github.com/akash-network/node/x/provider/types/v1beta2"
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

var (
	// NewKeeper creates new keeper instance of provider module
	NewKeeper = keeper.NewKeeper
)

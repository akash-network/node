package deployment

import (
	"github.com/akash-network/node/x/deployment/keeper"
	types "github.com/akash-network/node/x/deployment/types/v1beta2"
)

const (
	// StoreKey represents storekey of deployment module
	StoreKey = types.StoreKey
	// ModuleName represents current module name
	ModuleName = types.ModuleName
)

type (
	// Keeper defines keeper of deployment module
	Keeper = keeper.Keeper
)

var (
	// NewKeeper creates new keeper instance of deployment module
	NewKeeper = keeper.NewKeeper
)

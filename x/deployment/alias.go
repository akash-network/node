package deployment

import (
	"github.com/ovrclk/akash/x/deployment/keeper"
	"github.com/ovrclk/akash/x/deployment/types"
)

const (
	StoreKey   = types.StoreKey
	ModuleName = types.ModuleName
)

type (
	Keeper = keeper.Keeper
)

var (
	NewKeeper = keeper.NewKeeper
)

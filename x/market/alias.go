package market

import (
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
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

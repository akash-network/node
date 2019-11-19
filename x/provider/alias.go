package provider

import (
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
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

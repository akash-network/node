package verification

import (
	module "pkg.akt.dev/node/v2/x/verification/types"

	"pkg.akt.dev/node/v2/x/verification/keeper"
)

const (
	StoreKey   = module.StoreKey
	ModuleName = module.ModuleName
)

type Keeper = keeper.Keeper

var NewKeeper = keeper.NewKeeper

package keeper

import (
	types "pkg.akt.dev/go/node/escrow/v1"
)

func NewQuerier(k Keeper) types.QueryServer {
	return k.NewQuerier()
}

package keeper

import (
	types "pkg.akt.dev/go/node/escrow/v1"
)

func NewQuerier(_ Keeper) types.QueryServer {
	return nil
}

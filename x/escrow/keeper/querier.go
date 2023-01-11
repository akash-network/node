package keeper

import (
	types "github.com/akash-network/node/x/escrow/types/v1beta2"
)

func NewQuerier(k Keeper) types.QueryServer {
	return nil
}

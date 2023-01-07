package keeper

import (
	etypes "github.com/akash-network/node/x/escrow/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
}

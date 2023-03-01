package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
}

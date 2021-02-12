package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	etypes "github.com/ovrclk/akash/x/escrow/types"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
}

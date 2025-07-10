package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	etypes "pkg.akt.dev/go/node/escrow/v1"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
}

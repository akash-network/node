package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	etypes "github.com/ovrclk/akash/x/escrow/types"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
	GetPayment(ctx sdk.Context, id etypes.AccountID, pid string) (etypes.FractionalPayment, error)
	AccountClose(ctx sdk.Context, id etypes.AccountID) error
	PaymentClose(ctx sdk.Context, id etypes.AccountID, pid string) error
}

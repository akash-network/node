package keeper

import (
	etypes "github.com/akash-network/node/x/escrow/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
	GetPayment(ctx sdk.Context, id etypes.AccountID, pid string) (etypes.FractionalPayment, error)
	AccountClose(ctx sdk.Context, id etypes.AccountID) error
	PaymentClose(ctx sdk.Context, id etypes.AccountID, pid string) error
}

package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	v1 "pkg.akt.dev/go/node/escrow/v1"
)

type EscrowKeeper interface {
	GetAccount(ctx sdk.Context, id v1.AccountID) (v1.Account, error)
	GetPayment(ctx sdk.Context, id v1.AccountID, pid string) (v1.FractionalPayment, error)
	AccountClose(ctx sdk.Context, id v1.AccountID) error
	PaymentClose(ctx sdk.Context, id v1.AccountID, pid string) error
}

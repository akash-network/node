package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

//go:generate mockery --name BankKeeper --output ./mocks
type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx sdk.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx sdk.Context, senderModule string, recipientModule string, amt sdk.Coins) error
}

//go:generate mockery --name TakeKeeper --output ./mocks
type TakeKeeper interface {
	SubtractFees(ctx sdk.Context, amt sdk.Coin) (sdk.Coin, sdk.Coin, error)
}

//go:generate mockery --name DistrKeeper --output ./mocks
type DistrKeeper interface {
	GetFeePool(ctx sdk.Context) distrtypes.FeePool
	SetFeePool(ctx sdk.Context, pool distrtypes.FeePool)
}

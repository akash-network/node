package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

//go:generate mockery --name BankKeeper --output ./mocks
type BankKeeper interface {
	SendCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx sdk.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
}

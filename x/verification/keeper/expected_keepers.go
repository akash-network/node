package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
}

type ProviderKeeper interface {
	Get(ctx sdk.Context, id sdk.Address) (ptypes.Provider, bool)
}

type Option func(*keeper)

func WithAuthority(authority string) Option {
	return func(k *keeper) {
		k.authority = authority
	}
}

func WithBankKeeper(bank BankKeeper) Option {
	return func(k *keeper) {
		k.bank = bank
	}
}

func WithProviderKeeper(provider ProviderKeeper) Option {
	return func(k *keeper) {
		k.provider = provider
	}
}

package keeper

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	mv1 "pkg.akt.dev/go/node/market/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

type BankKeeper interface {
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
}

type ProviderKeeper interface {
	Get(ctx sdk.Context, id sdk.Address) (ptypes.Provider, bool)
	GetRegistrationTime(ctx sdk.Context, id sdk.Address) (time.Time, bool)
}

type MarketStatsKeeper interface {
	GetProviderLeaseStats(ctx sdk.Context, provider sdk.Address) (uint64, map[mv1.LeaseClosedReason]uint64, bool)
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

func WithMarketStatsKeeper(market MarketStatsKeeper) Option {
	return func(k *keeper) {
		k.market = market
	}
}

package handler

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
)

type ProviderKeeper interface {
	Get(ctx sdk.Context, id sdk.Address) (ptypes.Provider, bool)
	GetRegistrationTime(ctx sdk.Context, id sdk.Address) (time.Time, bool)
}

type MarketStatsKeeper interface {
	GetProviderLeaseStats(ctx sdk.Context, provider sdk.Address) (ProviderLeaseStats, bool)
}

type ProviderLeaseStats struct {
	Completed  uint64
	Terminated uint64
}

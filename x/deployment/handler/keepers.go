package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// MarketKeeper Interface includes market methods
type MarketKeeper interface {
	CreateOrder(ctx sdk.Context, id types.GroupID, spec types.GroupSpec) (mtypes.Order, error)
	OnGroupClosed(ctx sdk.Context, id types.GroupID)
}

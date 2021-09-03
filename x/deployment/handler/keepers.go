package handler

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	"github.com/ovrclk/akash/x/deployment/types"
	etypes "github.com/ovrclk/akash/x/escrow/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// MarketKeeper Interface includes market methods
type MarketKeeper interface {
	CreateOrder(ctx sdk.Context, id types.GroupID, spec types.GroupSpec) (mtypes.Order, error)
	OnGroupClosed(ctx sdk.Context, id types.GroupID)
}

type EscrowKeeper interface {
	AccountCreate(ctx sdk.Context, id etypes.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error
	AccountDeposit(ctx sdk.Context, id etypes.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error
	AccountClose(ctx sdk.Context, id etypes.AccountID) error
}

type AuthzKeeper interface {
	DeleteGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) error
	GetCleanAuthorization(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) (cap authz.Authorization, expiration time.Time)
	SaveGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration time.Time) error
}

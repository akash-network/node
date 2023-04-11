package handler

import (
	"time"

	types "github.com/akash-network/node/x/deployment/types/v1beta2"
	etypes "github.com/akash-network/node/x/escrow/types/v1beta2"
	mtypes "github.com/akash-network/node/x/market/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
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
	/// handle grant
	GetCleanAuthorization(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) (cap authz.Authorization, expiration time.Time)
	SaveGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration time.Time) error
}

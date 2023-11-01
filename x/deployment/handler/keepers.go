package handler

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"

	mtypes "github.com/akash-network/akash-api/go/node/market/v1beta4"
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

//go:generate mockery --name AuthzKeeper --output ./mocks
type AuthzKeeper interface {
	DeleteGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) error
	GetCleanAuthorization(ctx sdk.Context, grantee, granter sdk.AccAddress, msgType string) (cap authz.Authorization, expiration time.Time)
	SaveGrant(ctx sdk.Context, grantee, granter sdk.AccAddress, authorization authz.Authorization, expiration time.Time) error
}

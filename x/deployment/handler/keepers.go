package handler

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"

	types "pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
)

// MarketKeeper Interface includes market methods
type MarketKeeper interface {
	CreateOrder(ctx sdk.Context, id types.GroupID, spec v1beta4.GroupSpec) (mtypes.Order, error)
	OnGroupClosed(ctx sdk.Context, id types.GroupID) error
}

type EscrowKeeper interface {
	AccountCreate(ctx sdk.Context, id etypes.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error
	AccountDeposit(ctx sdk.Context, id etypes.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error
	AccountClose(ctx sdk.Context, id etypes.AccountID) error
	GetAccount(ctx sdk.Context, id etypes.AccountID) (etypes.Account, error)
}

//go:generate mockery --name AuthzKeeper --output ./mocks
type AuthzKeeper interface {
	DeleteGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) error
	GetAuthorization(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) (authz.Authorization, *time.Time)
	SaveGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
}

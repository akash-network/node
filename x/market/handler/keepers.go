package handler

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	atypes "pkg.akt.dev/go/node/audit/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	dbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/node/x/market/keeper"
)

type EscrowKeeper interface {
	AccountCreate(ctx sdk.Context, id etypes.AccountID, owner sdk.AccAddress, deposits []etypes.Deposit) error
	AccountDeposit(ctx sdk.Context, id etypes.AccountID, deposits []etypes.Deposit) error
	AccountClose(ctx sdk.Context, id etypes.AccountID) error
	PaymentCreate(ctx sdk.Context, id etypes.AccountID, pid string, provider sdk.AccAddress, rate sdk.DecCoin) error
	PaymentWithdraw(ctx sdk.Context, id etypes.AccountID, pid string) error
	PaymentClose(ctx sdk.Context, id etypes.AccountID, pid string) error
}

// ProviderKeeper Interface includes provider methods
type ProviderKeeper interface {
	Get(ctx sdk.Context, id sdk.Address) (ptypes.Provider, bool)
	WithProviders(ctx sdk.Context, fn func(ptypes.Provider) bool)
}

type AuditKeeper interface {
	GetProviderAttributes(ctx sdk.Context, id sdk.Address) (atypes.AuditedProviders, bool)
}

// DeploymentKeeper Interface includes deployment methods
type DeploymentKeeper interface {
	GetGroup(ctx sdk.Context, id dtypes.GroupID) (dbeta.Group, bool)
	OnBidClosed(ctx sdk.Context, id dtypes.GroupID) error
	OnLeaseClosed(ctx sdk.Context, id dtypes.GroupID) (dbeta.Group, error)
}

type AuthzKeeper interface {
	DeleteGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) error
	GetAuthorization(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) (authz.Authorization, *time.Time)
	SaveGrant(ctx context.Context, grantee sdk.AccAddress, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) error
	GetGranteeGrantsByMsgType(ctx context.Context, grantee sdk.AccAddress, msgType string, onGrant func(context.Context, sdk.AccAddress, authz.Authorization, *time.Time) bool)
}

type BankKeeper interface {
	SpendableCoins(ctx context.Context, addr sdk.AccAddress) sdk.Coins
	SpendableCoin(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// Keepers include all modules keepers
type Keepers struct {
	Escrow     EscrowKeeper
	Market     keeper.IKeeper
	Deployment DeploymentKeeper
	Provider   ProviderKeeper
	Audit      AuditKeeper
	Account    govtypes.AccountKeeper
	Authz      AuthzKeeper
	Bank       BankKeeper
}

package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	atypes "pkg.akt.dev/go/node/audit/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	dbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	etypes "pkg.akt.dev/go/node/escrow/v1"
	ptypes "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/akashd/x/market/keeper"
)

type EscrowKeeper interface {
	AccountCreate(ctx sdk.Context, id etypes.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error
	AccountDeposit(ctx sdk.Context, id etypes.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error
	AccountClose(ctx sdk.Context, id etypes.AccountID) error
	PaymentCreate(ctx sdk.Context, id etypes.AccountID, pid string, owner sdk.AccAddress, rate sdk.DecCoin) error
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

// Keepers include all modules keepers
type Keepers struct {
	Escrow     EscrowKeeper
	Market     keeper.IKeeper
	Deployment DeploymentKeeper
	Provider   ProviderKeeper
	Audit      AuditKeeper
	Account    govtypes.AccountKeeper
	Bank       bankkeeper.Keeper
}

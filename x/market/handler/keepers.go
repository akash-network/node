package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"

	atypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	etypes "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	"github.com/akash-network/node/x/market/keeper"
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
	GetProviderAttributes(ctx sdk.Context, id sdk.Address) (atypes.Providers, bool)
}

// DeploymentKeeper Interface includes deployment methods
type DeploymentKeeper interface {
	GetGroup(ctx sdk.Context, id dtypes.GroupID) (dtypes.Group, bool)
	OnBidClosed(ctx sdk.Context, id dtypes.GroupID) error
	OnLeaseClosed(ctx sdk.Context, id dtypes.GroupID) (dtypes.Group, error)
}

// Keepers include all modules keepers
type Keepers struct {
	Escrow     EscrowKeeper
	Market     keeper.IKeeper
	Deployment DeploymentKeeper
	Provider   ProviderKeeper
	Audit      AuditKeeper
	Bank       bankkeeper.Keeper
}

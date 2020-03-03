package handler

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/keeper"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

// ProviderKeeper Interface includes provider methods
type ProviderKeeper interface {
	Get(ctx sdk.Context, id sdk.Address) (ptypes.Provider, bool)
}

// DeploymentKeeper Interface includes deployment methods
type DeploymentKeeper interface {
	GetGroup(ctx sdk.Context, id dtypes.GroupID) (dtypes.Group, bool)
	OnLeaseCreated(ctx sdk.Context, id dtypes.GroupID)
	OnLeaseInsufficientFunds(ctx sdk.Context, id dtypes.GroupID)
	OnLeaseClosed(ctx sdk.Context, id dtypes.GroupID)
}

// Keepers include all modules keepers
type Keepers struct {
	Market     keeper.Keeper
	Deployment DeploymentKeeper
	Provider   ProviderKeeper
	Bank       bank.Keeper
}

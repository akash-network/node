package inflation

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/inflation/keeper"
	types "github.com/ovrclk/akash/x/inflation/types/v1beta2"
	abci "github.com/tendermint/tendermint/abci/types"
)

// ValidateGenesis does validation check of the Genesis and return error incase of failure
func ValidateGenesis(data *types.GenesisState) error {
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		Params: types.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	keeper.SetParams(ctx, data.Params)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	params := k.GetParams(ctx)
	return &types.GenesisState{
		Params: params,
	}
}

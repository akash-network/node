package wasm

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/oracle/v1"

	"pkg.akt.dev/node/v2/x/oracle/keeper"
)

// ValidateGenesis does validation check of the Genesis and return error incase of failure
func ValidateGenesis(data *types.GenesisState) error {
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the deployment
// module.
func DefaultGenesisState() *types.GenesisState {
	params := types.DefaultParams()

	return &types.GenesisState{
		Params: params,
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) {
	err := keeper.SetParams(ctx, data.Params)
	if err != nil {
		panic(err.Error())
	}
}

// ExportGenesis returns genesis state for the deployment module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	params := k.GetParams(ctx)
	return &types.GenesisState{
		Params: params,
	}
}

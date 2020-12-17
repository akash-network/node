package market

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// ValidateGenesis does validation check of the Genesis
func ValidateGenesis(data *types.GenesisState) error {
	return data.Params.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the market
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		Params: types.DefaultParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) []abci.ValidatorUpdate {
	keeper.SetParams(ctx, data.Params)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the market module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	params := k.GetParams(ctx)
	return &types.GenesisState{
		Params: params,
	}
}

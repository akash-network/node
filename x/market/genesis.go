package market

import (
	"github.com/akash-network/node/x/market/keeper"
	types "github.com/akash-network/node/x/market/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	keeper.SetParams(ctx, data.Params)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the market module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	params := k.GetParams(ctx)
	return &types.GenesisState{
		Params: params,
	}
}

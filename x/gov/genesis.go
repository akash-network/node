package provider

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	types "github.com/akash-network/akash-api/go/node/gov/v1beta3"

	"github.com/akash-network/node/x/gov/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure
func ValidateGenesis(data *types.GenesisState) error {
	return data.DepositParams.Validate()
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		DepositParams: types.DefaultDepositParams(),
	}
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	if err := keeper.SetDepositParams(ctx, data.DepositParams); err != nil {
		panic(err)
	}

	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	return &types.GenesisState{
		DepositParams: k.GetDepositParams(ctx),
	}
}

// GetGenesisStateFromAppState returns x/gov GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}

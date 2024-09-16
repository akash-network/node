package provider

import (
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/gov/v1beta3"

	"pkg.akt.dev/node/x/gov/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure
func ValidateGenesis(_ *types.GenesisState) error {
	// return data.DepositParams.Validate()
	return nil
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		// DepositParams: types.DefaultDepositParams(),
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

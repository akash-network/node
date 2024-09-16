package provider

import (
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/node/x/provider/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure
func ValidateGenesis(_ *types.GenesisState) error {
	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(_ sdk.Context, _ keeper.IKeeper, _ *types.GenesisState) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	var providers []types.Provider

	k.WithProviders(ctx, func(provider types.Provider) bool {
		providers = append(providers, provider)
		return true
	})

	return &types.GenesisState{
		Providers: providers,
	}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}

// GetGenesisStateFromAppState returns x/provider GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}

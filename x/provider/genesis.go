package provider

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/keeper"
	types "github.com/ovrclk/akash/x/provider/types/v1beta2"
	abci "github.com/tendermint/tendermint/abci/types"
)

// ValidateGenesis does validation check of the Genesis and returns error incase of failure
func ValidateGenesis(data *types.GenesisState) error {
	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.IKeeper, data *types.GenesisState) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	return &types.GenesisState{}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}

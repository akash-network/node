package provider

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/keeper"
	"github.com/ovrclk/akash/x/provider/types"
	abci "github.com/tendermint/tendermint/abci/types"
)

// GenesisState defines the basic genesis state used by provider module
type GenesisState struct {
	Providers []types.Provider `json:"providers"`
}

// ValidateGenesis does validation check of the Genesis and returns error incase of failure
func ValidateGenesis(data GenesisState) error {
	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data GenesisState) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) GenesisState {
	return GenesisState{}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() GenesisState {
	return GenesisState{}
}

package cert

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/x/cert/keeper"

	"github.com/ovrclk/akash/x/cert/types"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Certificates {
		if err := record.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Certificates {
		owner, err := sdk.AccAddressFromBech32(record.Owner)
		if err != nil {
			panic(fmt.Sprintf("error init certificate from genesis: %s", err))
		}

		err = keeper.CreateCertificate(ctx, owner, record.Certificate.Cert, record.Certificate.Pubkey)
		if err != nil {
			panic(fmt.Sprintf("error init certificate from genesis: %s", err))
		}
	}

	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	return &types.GenesisState{}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}

// GetGenesisStateFromAppState returns x/cert GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}

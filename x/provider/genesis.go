package provider

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/node/x/provider/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure

func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Providers {
		msg := &types.MsgCreateProvider{
			Owner:      record.Owner,
			HostURI:    record.HostURI,
			Attributes: record.Attributes,
			Info:       record.Info,
		}

		if err := msg.ValidateBasic(); err != nil {
			return err
		}

	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, kpr keeper.IKeeper, data *types.GenesisState) {
	store := ctx.KVStore(kpr.StoreKey())
	cdc := kpr.Codec()

	for _, record := range data.Providers {
		owner, err := sdk.AccAddressFromBech32(record.Owner)
		if err != nil {
			panic(fmt.Sprintf("provider genesis init: %s", err.Error()))
		}

		key := keeper.ProviderKey(owner)

		if store.Has(key) {
			panic(fmt.Sprintf("provider genesis init: %s", types.ErrProviderExists.Error()))
		}

		store.Set(key, cdc.MustMarshal(&record))
	}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	var providers []types.Provider

	k.WithProviders(ctx, func(provider types.Provider) bool {
		providers = append(providers, provider)
		return false
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

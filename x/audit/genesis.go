package audit

import (
	"encoding/json"
	"sort"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/akash-network/node/x/audit/keeper"

	types "github.com/akash-network/akash-api/go/node/audit/v1beta3"
)

// ValidateGenesis does validation check of the Genesis and returns error incase of failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Attributes {
		if _, err := sdk.AccAddressFromBech32(record.Owner); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid owner address")
		}

		if _, err := sdk.AccAddressFromBech32(record.Auditor); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid auditor address")
		}

		if err := record.Attributes.Validate(); err != nil {
			return sdkerrors.Wrap(err, "audited attributes: invalid attributes")
		}
	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, kpr keeper.Keeper, data *types.GenesisState) []abci.ValidatorUpdate {
	store := ctx.KVStore(kpr.StoreKey())
	cdc := kpr.Codec()

	for _, record := range data.Attributes {
		owner, err := sdk.AccAddressFromBech32(record.Owner)
		if err != nil {
			panic(sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid owner address").Error())
		}

		auditor, err := sdk.AccAddressFromBech32(record.Auditor)
		if err != nil {
			panic(sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid auditor address"))
		}

		key := keeper.ProviderKey(types.ProviderID{
			Owner:   owner,
			Auditor: auditor,
		})

		prov := types.Provider{
			Owner:      record.Owner,
			Auditor:    record.Auditor,
			Attributes: record.Attributes,
		}

		sort.SliceStable(prov.Attributes, func(i, j int) bool {
			return prov.Attributes[i].Key < prov.Attributes[j].Key
		})

		store.Set(key, cdc.MustMarshal(&prov))
	}

	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	var records []types.AuditedAttributes

	k.WithProviders(ctx, func(provider types.Provider) bool {
		records = append(records, types.AuditedAttributes{
			Owner:      provider.Owner,
			Auditor:    provider.Auditor,
			Attributes: provider.Attributes.Dup(),
		})
		return false
	})

	return &types.GenesisState{
		Attributes: records,
	}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{}
}

// GetGenesisStateFromAppState returns x/audit GenesisState given raw application
// genesis state.
func GetGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *types.GenesisState {
	var genesisState types.GenesisState

	if appState[ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ModuleName], &genesisState)
	}

	return &genesisState
}

package audit

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	abci "github.com/tendermint/tendermint/abci/types"

	types "github.com/akash-network/akash-api/go/node/audit/v1beta3"

	"github.com/akash-network/node/x/audit/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of a failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Attributes {
		if _, err := sdk.AccAddressFromBech32(record.Owner); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid owner address")
		}

		if _, err := sdk.AccAddressFromBech32(record.Auditor); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid auditor address")
		}

		if err := record.Attributes.Validate(); err != nil {
			sdkerrors.Wrap(err, "audited attributes: invalid attributes")
		}
	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) []abci.ValidatorUpdate {
	for _, record := range data.Attributes {
		owner, err := sdk.AccAddressFromBech32(record.Owner)

		if err != nil {
			panic(sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid owner address").Error())
		}

		auditor, err := sdk.AccAddressFromBech32(record.Auditor)
		if err != nil {
			panic(sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid auditor address"))
		}

		err = keeper.CreateOrUpdateProviderAttributes(ctx, types.ProviderID{
			Owner:   owner,
			Auditor: auditor,
		}, record.Attributes)
		if err != nil {
			panic(sdkerrors.Wrap(err, "unable to init genesis with provider"))
		}
	}

	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	var attr []types.AuditedAttributes

	k.WithProviders(ctx, func(provider types.Provider) bool {
		attr = append(attr, types.AuditedAttributes{
			Owner:      provider.Owner,
			Auditor:    provider.Auditor,
			Attributes: provider.Attributes.Dup(),
		})
		return false
	})

	return &types.GenesisState{}
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

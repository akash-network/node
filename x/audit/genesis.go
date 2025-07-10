package audit

import (
	"encoding/json"

	errorsmod "cosmossdk.io/errors"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	types "pkg.akt.dev/go/node/audit/v1"

	"pkg.akt.dev/node/x/audit/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in-case of failure
func ValidateGenesis(data *types.GenesisState) error {
	for _, record := range data.Providers {
		if _, err := sdk.AccAddressFromBech32(record.Owner); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid owner address")
		}

		if _, err := sdk.AccAddressFromBech32(record.Auditor); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrap("audited attributes: invalid auditor address")
		}

		if err := record.Attributes.Validate(); err != nil {
			return errorsmod.Wrap(err, "audited attributes: invalid attributes")
		}
	}

	return nil
}

// InitGenesis initiate genesis state and return updated validator details
func InitGenesis(ctx sdk.Context, keeper keeper.Keeper, data *types.GenesisState) {
	for _, record := range data.Providers {
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
			panic(errorsmod.Wrap(err, "unable to init genesis with provider"))
		}
	}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	var records []types.AuditedProvider

	k.WithProviders(ctx, func(provider types.AuditedProvider) bool {
		records = append(records, types.AuditedProvider{
			Owner:      provider.Owner,
			Auditor:    provider.Auditor,
			Attributes: provider.Attributes.Dup(),
		})
		return false
	})

	return &types.GenesisState{
		Providers: records,
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

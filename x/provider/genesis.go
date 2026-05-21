package provider

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/provider/v1beta4"

	"pkg.akt.dev/node/v2/x/provider/keeper"
)

// ValidateGenesis does validation check of the Genesis and returns error in case of failure

func ValidateGenesis(data *types.GenesisState) error {
	providers := make(map[string]struct{})
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

		if _, exists := providers[record.Owner]; exists {
			return types.ErrProviderExists.Wrapf("id: %s", record.Owner)
		}
		providers[record.Owner] = struct{}{}
	}

	params := data.Params
	if params == (types.ProviderMaintenanceParams{}) {
		params = keeper.DefaultParams()
	}

	if err := keeper.ValidateParams(params); err != nil {
		return err
	}

	registrations := make(map[string]struct{})
	for _, record := range data.Registrations {
		if err := validateGenesisRegistration(record, providers); err != nil {
			return err
		}

		if _, exists := registrations[record.Owner]; exists {
			return types.ErrProviderExists.Wrapf("duplicate registration: %s", record.Owner)
		}
		registrations[record.Owner] = struct{}{}
	}

	var maxMaintenanceID uint64
	maintenanceIDs := make(map[uint64]struct{})
	for _, record := range data.Maintenances {
		if err := validateGenesisMaintenance(record, providers, params); err != nil {
			return err
		}

		if _, exists := maintenanceIDs[record.ID]; exists {
			return sdkerrors.ErrInvalidRequest.Wrapf("duplicate maintenance id: %d", record.ID)
		}
		maintenanceIDs[record.ID] = struct{}{}

		if record.ID > maxMaintenanceID {
			maxMaintenanceID = record.ID
		}
	}

	if maxMaintenanceID > 0 && data.NextMaintenanceID <= maxMaintenanceID {
		return sdkerrors.ErrInvalidRequest.Wrap("next maintenance id must exceed imported maintenance ids")
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

	params := data.Params
	if params == (types.ProviderMaintenanceParams{}) {
		params = keeper.DefaultParams()
	}

	if err := kpr.SetParams(ctx, params); err != nil {
		panic(err.Error())
	}

	registered := make(map[string]struct{})
	for _, record := range data.Registrations {
		if err := kpr.SetRegistration(ctx, record); err != nil {
			panic(fmt.Sprintf("provider registration genesis init: %s", err.Error()))
		}
		registered[record.Owner] = struct{}{}
	}

	for _, record := range data.Providers {
		if _, exists := registered[record.Owner]; exists {
			continue
		}

		if err := kpr.SetRegistration(ctx, types.ProviderRegistration{
			Owner:        record.Owner,
			RegisteredAt: ctx.BlockTime(),
		}); err != nil {
			panic(fmt.Sprintf("provider registration genesis init: %s", err.Error()))
		}
	}

	nextMaintenanceID := data.NextMaintenanceID
	if nextMaintenanceID == 0 {
		nextMaintenanceID = 1
	}
	kpr.SetNextMaintenanceID(ctx, nextMaintenanceID)

	activeMaintenances := make(map[string]struct{})
	for _, record := range data.Maintenances {
		if err := kpr.SetMaintenance(ctx, record); err != nil {
			panic(fmt.Sprintf("provider maintenance genesis init: %s", err.Error()))
		}

		status := keeper.MaintenanceStatus(ctx.BlockTime(), record)
		if status == types.ProviderMaintenanceStatus_provider_maintenance_status_scheduled ||
			status == types.ProviderMaintenanceStatus_provider_maintenance_status_active {
			if _, exists := activeMaintenances[record.Provider]; exists {
				panic(fmt.Sprintf("provider maintenance genesis init: duplicate active maintenance for provider %s", record.Provider))
			}
			activeMaintenances[record.Provider] = struct{}{}

			provider, err := sdk.AccAddressFromBech32(record.Provider)
			if err != nil {
				panic(fmt.Sprintf("provider maintenance genesis init: %s", err.Error()))
			}
			kpr.SetActiveMaintenanceID(ctx, provider, record.ID)
		}
	}
}

// ExportGenesis returns genesis state as raw bytes for the provider module
func ExportGenesis(ctx sdk.Context, k keeper.IKeeper) *types.GenesisState {
	var providers []types.Provider
	var registrations []types.ProviderRegistration
	var maintenances []types.ProviderMaintenanceRecord

	k.WithProviders(ctx, func(provider types.Provider) bool {
		providers = append(providers, provider)
		return false
	})

	k.WithRegistrations(ctx, func(registration types.ProviderRegistration) bool {
		registrations = append(registrations, registration)
		return false
	})

	k.WithAllMaintenances(ctx, func(record types.ProviderMaintenanceRecord) bool {
		maintenances = append(maintenances, record)
		return false
	})

	return &types.GenesisState{
		Providers:         providers,
		Params:            k.GetParams(ctx),
		Maintenances:      maintenances,
		NextMaintenanceID: k.GetNextMaintenanceID(ctx),
		Registrations:     registrations,
	}
}

// DefaultGenesisState returns default genesis state as raw bytes for the provider
// module.
func DefaultGenesisState() *types.GenesisState {
	return &types.GenesisState{
		Params:            keeper.DefaultParams(),
		NextMaintenanceID: 1,
	}
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

func validateGenesisRegistration(record types.ProviderRegistration, providers map[string]struct{}) error {
	if _, err := sdk.AccAddressFromBech32(record.Owner); err != nil {
		return types.ErrInvalidAddress.Wrap(err.Error())
	}

	if record.RegisteredAt.IsZero() {
		return sdkerrors.ErrInvalidRequest.Wrap("registered_at must be set")
	}

	if _, exists := providers[record.Owner]; !exists {
		return types.ErrProviderNotFound.Wrapf("registration provider: %s", record.Owner)
	}

	return nil
}

func validateGenesisMaintenance(record types.ProviderMaintenanceRecord, providers map[string]struct{}, params types.ProviderMaintenanceParams) error {
	if record.ID == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance id must be set")
	}

	if _, err := sdk.AccAddressFromBech32(record.Provider); err != nil {
		return types.ErrInvalidAddress.Wrap(err.Error())
	}

	if _, exists := providers[record.Provider]; !exists {
		return types.ErrProviderNotFound.Wrapf("maintenance provider: %s", record.Provider)
	}

	if !keeper.ValidMaintenanceType(record.MaintenanceType) {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance type must be specified")
	}

	if record.StartsAt.IsZero() {
		return sdkerrors.ErrInvalidRequest.Wrap("starts_at must be set")
	}

	if record.ExpectedEndsAt.IsZero() {
		return sdkerrors.ErrInvalidRequest.Wrap("expected_ends_at must be set")
	}

	if !record.ExpectedEndsAt.After(record.StartsAt) {
		return sdkerrors.ErrInvalidRequest.Wrap("expected_ends_at must be after starts_at")
	}

	if record.ExpectedEndsAt.Sub(record.StartsAt) > params.MaintenanceMaxDuration {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance duration exceeds max duration")
	}

	if record.OpenedAt.IsZero() {
		return sdkerrors.ErrInvalidRequest.Wrap("opened_at must be set")
	}

	if record.ClosedAt != nil && record.ClosedAt.Before(record.OpenedAt) {
		return sdkerrors.ErrInvalidRequest.Wrap("closed_at must be after opened_at")
	}

	return nil
}

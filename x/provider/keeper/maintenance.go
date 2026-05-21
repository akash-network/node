package keeper

import (
	"time"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	types "pkg.akt.dev/go/node/provider/v1beta4"
)

func (k Keeper) GetParams(ctx sdk.Context) types.ProviderMaintenanceParams {
	store := ctx.KVStore(k.skey)
	bz := store.Get(providerParamsKey)
	if bz == nil {
		return DefaultParams()
	}

	var params types.ProviderMaintenanceParams
	k.cdc.MustUnmarshal(bz, &params)
	return params
}

func (k Keeper) SetParams(ctx sdk.Context, params types.ProviderMaintenanceParams) error {
	if err := ValidateParams(params); err != nil {
		return err
	}

	store := ctx.KVStore(k.skey)
	store.Set(providerParamsKey, k.cdc.MustMarshal(&params))
	return nil
}

func (k Keeper) GetMaintenance(ctx sdk.Context, id uint64) (types.ProviderMaintenanceRecord, bool) {
	store := ctx.KVStore(k.skey)
	key := ProviderMaintenanceKey(id)

	if !store.Has(key) {
		return types.ProviderMaintenanceRecord{}, false
	}

	var val types.ProviderMaintenanceRecord
	k.cdc.MustUnmarshal(store.Get(key), &val)
	return val, true
}

func (k Keeper) SetMaintenance(ctx sdk.Context, record types.ProviderMaintenanceRecord) error {
	if record.ID == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("maintenance id must be set")
	}

	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return types.ErrInvalidAddress.Wrap(err.Error())
	}

	store := ctx.KVStore(k.skey)
	if existing, found := k.GetMaintenance(ctx, record.ID); found && existing.Provider != record.Provider {
		previousProvider, err := sdk.AccAddressFromBech32(existing.Provider)
		if err != nil {
			return types.ErrInvalidAddress.Wrap(err.Error())
		}

		store.Delete(ProviderMaintenanceOwnerKey(previousProvider, record.ID))
	}

	store.Set(ProviderMaintenanceKey(record.ID), k.cdc.MustMarshal(&record))
	store.Set(ProviderMaintenanceOwnerKey(provider, record.ID), []byte{})
	return nil
}

func (k Keeper) WithMaintenances(ctx sdk.Context, provider sdk.Address, fn func(types.ProviderMaintenanceRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), ProviderMaintenanceOwnerPrefix(provider))

	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		if len(iter.Key()) != 8 {
			continue
		}

		record, found := k.GetMaintenance(ctx, uint64FromBytes(iter.Key()))
		if !found {
			continue
		}

		if stop := fn(record); stop {
			break
		}
	}
}

func (k Keeper) WithAllMaintenances(ctx sdk.Context, fn func(types.ProviderMaintenanceRecord) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), providerMaintenancePrefix)

	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.ProviderMaintenanceRecord
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k Keeper) GetActiveMaintenanceID(ctx sdk.Context, id sdk.Address) (uint64, bool) {
	store := ctx.KVStore(k.skey)
	bz := store.Get(ProviderActiveMaintenanceKey(id))
	if bz == nil {
		return 0, false
	}

	return uint64FromBytes(bz), true
}

func (k Keeper) SetActiveMaintenanceID(ctx sdk.Context, id sdk.Address, maintenanceID uint64) {
	store := ctx.KVStore(k.skey)
	store.Set(ProviderActiveMaintenanceKey(id), uint64Bytes(maintenanceID))
}

func (k Keeper) DeleteActiveMaintenanceID(ctx sdk.Context, id sdk.Address) {
	store := ctx.KVStore(k.skey)
	store.Delete(ProviderActiveMaintenanceKey(id))
}

func (k Keeper) GetNextMaintenanceID(ctx sdk.Context) uint64 {
	store := ctx.KVStore(k.skey)
	bz := store.Get(providerNextMaintenanceIDKey)
	if bz == nil {
		return 1
	}

	return uint64FromBytes(bz)
}

func (k Keeper) SetNextMaintenanceID(ctx sdk.Context, id uint64) {
	store := ctx.KVStore(k.skey)
	store.Set(providerNextMaintenanceIDKey, uint64Bytes(id))
}

func (k Keeper) AllocateMaintenanceID(ctx sdk.Context) uint64 {
	id := k.GetNextMaintenanceID(ctx)
	k.SetNextMaintenanceID(ctx, id+1)
	return id
}

func MaintenanceStatus(blockTime time.Time, record types.ProviderMaintenanceRecord) types.ProviderMaintenanceStatus {
	if record.ClosedAt != nil {
		return types.ProviderMaintenanceStatus_provider_maintenance_status_closed
	}

	if blockTime.Before(record.StartsAt) {
		return types.ProviderMaintenanceStatus_provider_maintenance_status_scheduled
	}

	if !blockTime.Before(record.ExpectedEndsAt) {
		return types.ProviderMaintenanceStatus_provider_maintenance_status_elapsed
	}

	return types.ProviderMaintenanceStatus_provider_maintenance_status_active
}

func ValidMaintenanceType(maintenanceType types.ProviderMaintenanceType) bool {
	switch maintenanceType {
	case types.ProviderMaintenanceType_provider_maintenance_type_planned,
		types.ProviderMaintenanceType_provider_maintenance_type_emergency,
		types.ProviderMaintenanceType_provider_maintenance_type_security,
		types.ProviderMaintenanceType_provider_maintenance_type_network,
		types.ProviderMaintenanceType_provider_maintenance_type_capacity:
		return true
	default:
		return false
	}
}

func ValidMaintenanceStatusFilter(status types.ProviderMaintenanceStatus) bool {
	switch status {
	case types.ProviderMaintenanceStatus_provider_maintenance_status_unspecified,
		types.ProviderMaintenanceStatus_provider_maintenance_status_scheduled,
		types.ProviderMaintenanceStatus_provider_maintenance_status_active,
		types.ProviderMaintenanceStatus_provider_maintenance_status_elapsed,
		types.ProviderMaintenanceStatus_provider_maintenance_status_closed:
		return true
	default:
		return false
	}
}

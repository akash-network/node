package keeper

import (
	"time"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/provider/v1beta4"
)

func (k Keeper) GetRegistration(ctx sdk.Context, id sdk.Address) (types.ProviderRegistration, bool) {
	store := ctx.KVStore(k.skey)
	key := ProviderRegistrationKey(id)

	if !store.Has(key) {
		return types.ProviderRegistration{}, false
	}

	var val types.ProviderRegistration
	k.cdc.MustUnmarshal(store.Get(key), &val)
	return val, true
}

func (k Keeper) GetRegistrationTime(ctx sdk.Context, id sdk.Address) (time.Time, bool) {
	registration, found := k.GetRegistration(ctx, id)
	if !found {
		return time.Time{}, false
	}

	return registration.RegisteredAt, true
}

func (k Keeper) SetRegistration(ctx sdk.Context, registration types.ProviderRegistration) error {
	owner, err := sdk.AccAddressFromBech32(registration.Owner)
	if err != nil {
		return types.ErrInvalidAddress.Wrap(err.Error())
	}

	store := ctx.KVStore(k.skey)
	store.Set(ProviderRegistrationKey(owner), k.cdc.MustMarshal(&registration))
	return nil
}

func (k Keeper) WithRegistrations(ctx sdk.Context, fn func(types.ProviderRegistration) bool) {
	store := prefix.NewStore(ctx.KVStore(k.skey), providerRegistrationPrefix)

	iter := store.Iterator(nil, nil)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.ProviderRegistration
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

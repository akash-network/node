package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/types"
)

type Keeper struct {
	skey sdk.StoreKey
	cdc  *codec.Codec
}

func NewKeeper(cdc *codec.Codec, skey sdk.StoreKey) Keeper {
	return Keeper{
		skey: skey,
		cdc:  cdc,
	}
}

func (k Keeper) Codec() *codec.Codec {
	return k.cdc
}

func (k Keeper) Get(ctx sdk.Context, id sdk.Address) (types.Provider, bool) {
	store := ctx.KVStore(k.skey)
	key := providerKey(id)

	if !store.Has(key) {
		return types.Provider{}, false
	}

	buf := store.Get(key)
	var val types.Provider
	k.cdc.MustUnmarshalBinaryBare(buf, &val)
	return val, true
}

func (k Keeper) Create(ctx sdk.Context, provider types.Provider) error {
	store := ctx.KVStore(k.skey)
	key := providerKey(provider.Owner)

	if store.Has(key) {
		return fmt.Errorf("provider already exists")
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(provider))
	return nil
}

func (k Keeper) WithProviders(ctx sdk.Context, fn func(types.Provider) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)
	for ; iter.Valid(); iter.Next() {
		var val types.Provider
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k Keeper) Update(ctx sdk.Context, provider types.Provider) error {
	store := ctx.KVStore(k.skey)
	key := providerKey(provider.Owner)

	if store.Has(key) {
		return fmt.Errorf("provider not found")
	}
	store.Set(key, k.cdc.MustMarshalBinaryBare(provider))
	return nil
}

func (k Keeper) Delete(ctx sdk.Context, id sdk.Address) {
	panic("TODO")
}

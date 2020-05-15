package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/provider/types"
)

var (
	ErrProviderAlreadyExists = errors.New("keeper: provider already exists")
	ErrProviderNotFound      = errors.New("keeper: provider not found")
)

// Keeper of the provider store
type Keeper struct {
	skey sdk.StoreKey
	cdc  *codec.Codec
}

// NewKeeper creates and returns an instance for Provider keeper
func NewKeeper(cdc *codec.Codec, skey sdk.StoreKey) Keeper {
	return Keeper{
		skey: skey,
		cdc:  cdc,
	}
}

// Codec returns keeper codec
func (k Keeper) Codec() *codec.Codec {
	return k.cdc
}

// Get returns a provider with given provider id
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

// Create creates a new provider or returns an error if the provider exists already
func (k Keeper) Create(ctx sdk.Context, provider types.Provider) error {
	store := ctx.KVStore(k.skey)
	key := providerKey(provider.Owner)

	if store.Has(key) {
		return ErrProviderAlreadyExists
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(provider))
	return nil
}

// WithProviders iterates all providers
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

// Update updates a provider details
func (k Keeper) Update(ctx sdk.Context, provider types.Provider) error {
	store := ctx.KVStore(k.skey)
	key := providerKey(provider.Owner)

	if !store.Has(key) {
		return ErrProviderNotFound
	}
	store.Set(key, k.cdc.MustMarshalBinaryBare(provider))
	return nil
}

// Delete delete a provider
func (k Keeper) Delete(ctx sdk.Context, id sdk.Address) {
	panic("TODO")
}

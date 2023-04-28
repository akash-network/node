package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/akash-network/akash-api/go/node/provider/v1beta3"
)

type IKeeper interface {
	Codec() codec.BinaryCodec
	StoreKey() sdk.StoreKey
	Get(ctx sdk.Context, id sdk.Address) (types.Provider, bool)
	Create(ctx sdk.Context, provider types.Provider) error
	WithProviders(ctx sdk.Context, fn func(types.Provider) bool)
	Update(ctx sdk.Context, provider types.Provider) error
	Delete(ctx sdk.Context, id sdk.Address)
	NewQuerier() Querier
}

// Keeper of the provider store
type Keeper struct {
	skey sdk.StoreKey
	cdc  codec.BinaryCodec
}

// NewKeeper creates and returns an instance for Provider keeper
func NewKeeper(cdc codec.BinaryCodec, skey sdk.StoreKey) IKeeper {
	return Keeper{
		skey: skey,
		cdc:  cdc,
	}
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k Keeper) StoreKey() sdk.StoreKey {
	return k.skey
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
	k.cdc.MustUnmarshal(buf, &val)
	return val, true
}

// Create creates a new provider or returns an error if the provider exists already
func (k Keeper) Create(ctx sdk.Context, provider types.Provider) error {
	store := ctx.KVStore(k.skey)
	owner, err := sdk.AccAddressFromBech32(provider.Owner)
	if err != nil {
		return err
	}

	key := providerKey(owner)

	if store.Has(key) {
		return types.ErrProviderExists
	}

	store.Set(key, k.cdc.MustMarshal(&provider))

	ctx.EventManager().EmitEvent(
		types.EventProviderCreated{Owner: owner}.ToSDKEvent(),
	)

	return nil
}

// WithProviders iterates all providers
func (k Keeper) WithProviders(ctx sdk.Context, fn func(types.Provider) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Provider
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// Update updates a provider details
func (k Keeper) Update(ctx sdk.Context, provider types.Provider) error {
	store := ctx.KVStore(k.skey)
	owner, err := sdk.AccAddressFromBech32(provider.Owner)
	if err != nil {
		return err
	}

	key := providerKey(owner)

	if !store.Has(key) {
		return types.ErrProviderNotFound
	}
	store.Set(key, k.cdc.MustMarshal(&provider))

	ctx.EventManager().EmitEvent(
		types.EventProviderUpdated{Owner: owner}.ToSDKEvent(),
	)

	return nil
}

// Delete delete a provider
func (k Keeper) Delete(ctx sdk.Context, id sdk.Address) {
	panic("TODO")
}

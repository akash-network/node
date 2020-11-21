package keeper

import (
	"sort"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	akashtypes "github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/x/audit/types"
)

// Keeper of the provider store
type Keeper struct {
	skey sdk.StoreKey
	cdc  codec.BinaryMarshaler
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryMarshaler, skey sdk.StoreKey) Keeper {
	return Keeper{cdc: cdc, skey: skey}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryMarshaler {
	return k.cdc
}

// GetProviderByValidator returns a provider with given validator and owner id
func (k Keeper) GetProviderByValidator(ctx sdk.Context, id types.ProviderID) (types.Provider, bool) {
	store := ctx.KVStore(k.skey)

	buf := store.Get(providerKey(id))
	if buf == nil {
		return types.Provider{}, false
	}

	var val types.Provider
	k.cdc.MustUnmarshalBinaryBare(buf, &val)

	return val, true
}

// GetProviderAttributes returns a provider with given validator and owner id's
func (k Keeper) GetProviderAttributes(ctx sdk.Context, id sdk.Address) (types.Providers, bool) {
	store := ctx.KVStore(k.skey)

	var attr types.Providers

	iter := sdk.KVStorePrefixIterator(store, providerPrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Provider
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		attr = append(attr, val)
	}

	if len(attr) == 0 {
		return nil, false
	}

	return attr, true
}

// CreateOrUpdateProviderAttributes update signed provider attributes.
// creates new if key does not exist
// if key exists, existing values for matching pairs will be replaced
func (k Keeper) CreateOrUpdateProviderAttributes(ctx sdk.Context, id types.ProviderID, attr akashtypes.Attributes) error {
	store := ctx.KVStore(k.skey)
	key := providerKey(id)

	prov := types.Provider{
		Owner:      id.Owner.String(),
		Validator:  id.Validator.String(),
		Attributes: attr,
	}

	buf := store.Get(key)
	if buf != nil {
		tmp := types.Provider{}
		k.cdc.MustUnmarshalBinaryBare(buf, &tmp)

		kv := make(map[string]string)

		for _, entry := range tmp.Attributes {
			kv[entry.Key] = entry.Value
		}

		for _, entry := range prov.Attributes {
			kv[entry.Key] = entry.Value
		}

		attr = akashtypes.Attributes{}

		for ky, val := range kv {
			attr = append(attr, atypes.Attribute{
				Key:   ky,
				Value: val,
			})
		}

		prov.Attributes = attr
	}

	sort.SliceStable(prov.Attributes, func(i, j int) bool {
		return prov.Attributes[i].Key < prov.Attributes[j].Key
	})

	store.Set(key, k.cdc.MustMarshalBinaryBare(&prov))
	return nil
}

func (k Keeper) DeleteProviderAttributes(ctx sdk.Context, id types.ProviderID, keys []string) error {
	store := ctx.KVStore(k.skey)
	key := providerKey(id)

	buf := store.Get(key)
	if buf == nil {
		return types.ErrProviderNotFound
	}

	if keys == nil {
		store.Delete(key)
	} else {
		prov := types.Provider{
			Owner:     id.Owner.String(),
			Validator: id.Validator.String(),
		}

		tmp := types.Provider{}
		k.cdc.MustUnmarshalBinaryBare(buf, &tmp)

		kv := make(map[string]string)

		for _, entry := range tmp.Attributes {
			kv[entry.Key] = entry.Value
		}

		for _, entry := range keys {
			if _, exists := kv[entry]; !exists {
				return errors.Errorf("trying to delete non-existing attribute \"%s\" for validator/provider \"%s/%s\"",
					entry,
					prov.Validator,
					prov.Owner)
			}

			delete(kv, entry)
		}

		var attr akashtypes.Attributes

		for ky, val := range kv {
			attr = append(attr, atypes.Attribute{
				Key:   ky,
				Value: val,
			})
		}

		if len(attr) == 0 {
			store.Delete(key)
		} else {
			sort.SliceStable(attr, func(i, j int) bool {
				return attr[i].Key < attr[j].Key
			})

			prov.Attributes = attr

			store.Set(key, k.cdc.MustMarshalBinaryBare(&prov))
		}
	}

	return nil
}

// WithProviders iterates all signed provider's attributes
func (k Keeper) WithProviders(ctx sdk.Context, fn func(types.Provider) bool) {
	store := ctx.KVStore(k.skey)
	iter := store.Iterator(nil, nil)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Provider
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// WithProviders iterates all signed provider's attributes
func (k Keeper) WithProvider(ctx sdk.Context, id sdk.Address, fn func(types.Provider) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, providerPrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Provider
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

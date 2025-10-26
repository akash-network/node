package keeper

import (
	"sort"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/audit/v1"
	attrv1 "pkg.akt.dev/go/node/types/attributes/v1"
)

type IKeeper interface {
	GetProviderByAuditor(ctx sdk.Context, id types.ProviderID) (types.AuditedProvider, bool)
	GetProviderAttributes(ctx sdk.Context, id sdk.Address) (types.AuditedProviders, bool)
	CreateOrUpdateProviderAttributes(ctx sdk.Context, id types.ProviderID, attr attrv1.Attributes) error
	DeleteProviderAttributes(ctx sdk.Context, id types.ProviderID, keys []string) error
	WithProviders(ctx sdk.Context, fn func(types.AuditedProvider) bool)
	WithProvider(ctx sdk.Context, id sdk.Address, fn func(types.AuditedProvider) bool)
}

// Keeper of the provider store
type Keeper struct {
	skey storetypes.StoreKey
	cdc  codec.BinaryCodec
}

// NewKeeper creates and returns an instance for Market keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey) Keeper {
	return Keeper{cdc: cdc, skey: skey}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

// GetProviderByAuditor returns a provider with given auditor and owner id
func (k Keeper) GetProviderByAuditor(ctx sdk.Context, id types.ProviderID) (types.AuditedProvider, bool) {
	store := ctx.KVStore(k.skey)

	buf := store.Get(ProviderKey(id))
	if buf == nil {
		return types.AuditedProvider{}, false
	}

	var sVal types.AuditedAttributesStore
	k.cdc.MustUnmarshal(buf, &sVal)

	return types.AuditedProvider{
		Owner:      id.Owner.String(),
		Auditor:    id.Auditor.String(),
		Attributes: sVal.Attributes,
	}, true
}

// GetProviderAttributes returns a provider with given auditor and owner id's
func (k Keeper) GetProviderAttributes(ctx sdk.Context, id sdk.Address) (types.AuditedProviders, bool) {
	store := ctx.KVStore(k.skey)

	var res types.AuditedProviders

	prefix := ProviderPrefix(id)
	iter := storetypes.KVStorePrefixIterator(store, prefix)
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		aID := ParseIDFromKey(iter.Key())

		var sVal types.AuditedAttributesStore
		k.cdc.MustUnmarshal(iter.Value(), &sVal)
		res = append(res, types.AuditedProvider{
			Owner:      id.String(),
			Auditor:    aID.Auditor.String(),
			Attributes: sVal.Attributes,
		})
	}

	if len(res) == 0 {
		return nil, false
	}

	return res, true
}

// CreateOrUpdateProviderAttributes update signed provider attributes.
// creates new if key does not exist
// if key exists, existing values for matching pairs will be replaced
func (k Keeper) CreateOrUpdateProviderAttributes(ctx sdk.Context, id types.ProviderID, attr attrv1.Attributes) error {
	store := ctx.KVStore(k.skey)
	key := ProviderKey(id)

	attrRec := types.AuditedAttributesStore{
		Attributes: attr,
	}

	buf := store.Get(key)
	if buf != nil {
		tmp := types.AuditedAttributesStore{}
		k.cdc.MustUnmarshal(buf, &tmp)

		kv := make(map[string]string)

		for _, entry := range tmp.Attributes {
			kv[entry.Key] = entry.Value
		}

		for _, entry := range attrRec.Attributes {
			kv[entry.Key] = entry.Value
		}

		attr = attrv1.Attributes{}

		for ky, val := range kv {
			attr = append(attr, attrv1.Attribute{
				Key:   ky,
				Value: val,
			})
		}

		attrRec.Attributes = attr
	}

	sort.Stable(attrRec.Attributes)

	store.Set(key, k.cdc.MustMarshal(&attrRec))

	err := ctx.EventManager().EmitTypedEvent(
		&types.EventTrustedAuditorCreated{
			Owner:   id.Owner.String(),
			Auditor: id.Auditor.String(),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (k Keeper) DeleteProviderAttributes(ctx sdk.Context, id types.ProviderID, keys []string) error {
	store := ctx.KVStore(k.skey)
	key := ProviderKey(id)

	buf := store.Get(key)
	if buf == nil {
		return types.ErrProviderNotFound
	}

	if keys == nil {
		store.Delete(key)
	} else {
		prov := types.AuditedAttributesStore{}

		tmp := types.AuditedAttributesStore{}
		k.cdc.MustUnmarshal(buf, &tmp)

		kv := make(map[string]string)

		for _, entry := range tmp.Attributes {
			kv[entry.Key] = entry.Value
		}

		for _, entry := range keys {
			if _, exists := kv[entry]; !exists {
				return types.ErrAttributeNotFound.Wrapf("trying to delete non-existing attribute \"%s\" for auditor/provider \"%s/%s\"",
					entry,
					id.Auditor,
					id.Owner)
			}

			delete(kv, entry)
		}

		var attr attrv1.Attributes

		for ky, val := range kv {
			attr = append(attr, attrv1.Attribute{
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

			store.Set(key, k.cdc.MustMarshal(&prov))
		}
	}

	err := ctx.EventManager().EmitTypedEvent(
		&types.EventTrustedAuditorDeleted{
			Owner:   id.Owner.String(),
			Auditor: id.Auditor.String(),
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// WithProviders iterates all signed provider's attributes
func (k Keeper) WithProviders(ctx sdk.Context, fn func(types.AuditedProvider) bool) {
	store := ctx.KVStore(k.skey)

	iter := storetypes.KVStorePrefixIterator(store, types.PrefixProviderID())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id := ParseIDFromKey(iter.Key())

		var attr types.AuditedAttributesStore
		k.cdc.MustUnmarshal(iter.Value(), &attr)

		val := types.AuditedProvider{
			Owner:      id.Owner.String(),
			Auditor:    id.Auditor.String(),
			Attributes: attr.Attributes,
		}

		if stop := fn(val); stop {
			break
		}
	}
}

// WithProvider returns requested signed provider attributes
func (k Keeper) WithProvider(ctx sdk.Context, id sdk.Address, fn func(types.AuditedProvider) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, ProviderPrefix(id))
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		aID := ParseIDFromKey(iter.Key())

		var attr types.AuditedAttributesStore
		k.cdc.MustUnmarshal(iter.Value(), &attr)

		val := types.AuditedProvider{
			Owner:      id.String(),
			Auditor:    aID.Auditor.String(),
			Attributes: attr.Attributes,
		}
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

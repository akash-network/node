package v015

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	v015 "github.com/ovrclk/akash/util/legacy/v015"
	dmigrate "github.com/ovrclk/akash/x/deployment/types/v1beta2/migrate"
	"github.com/ovrclk/akash/x/market/keeper/keys"
	"github.com/ovrclk/akash/x/market/types/v1beta1"
	types "github.com/ovrclk/akash/x/market/types/v1beta2"
)

// MigrateStore performs in-place store migrations from v0.14 to v0.15. The
// migration includes:
//
// - Change addresses to be length-prefixed
// - Migrating Order proto from v1beta1 to v1beta2
// - Migrating Bid proto from v1beta1 to v1beta2
// - Migrating Lease proto from v1beta1 to v1beta2
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey, cdc codec.BinaryCodec) error {
	store := ctx.KVStore(storeKey)
	// Change addresses to be length-prefixed
	v015.MigratePrefixBech32AddrBytes(store, types.OrderPrefix())
	migratePrefixBech32Uint64Uint32Uint32Bech32(store, types.BidPrefix())
	migratePrefixBech32Uint64Uint32Uint32Bech32(store, types.LeasePrefix())

	// Migrate protobuf
	v015.MigrateValue(store, cdc, types.OrderPrefix(), migrateOrder)
	v015.MigrateValue(store, cdc, types.BidPrefix(), migrateBid)
	v015.MigrateValue(store, cdc, types.LeasePrefix(), migrateLease)

	// add the mapping of secondary lease key -> lease key
	addSecondaryLeaseKeys(store, cdc)

	return nil
}

func addSecondaryLeaseKeys(baseStore sdk.KVStore, cdc codec.BinaryCodec) {
	store := prefix.NewStore(baseStore, types.LeasePrefix())

	iter := sdk.KVStorePrefixIterator(store, types.LeasePrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var lease types.Lease
		cdc.MustUnmarshal(iter.Value(), &lease)

		leaseKey := keys.LeaseKey(lease.GetLeaseID())
		for _, secondaryKey := range keys.SecondaryKeysForLease(lease.GetLeaseID()) {
			store.Set(secondaryKey, leaseKey)
		}
	}
}

// migratePrefixBech32Uint64Uint32Uint32Bech32 is a helper function that migrates all keys of format:
// prefix_bytes | address1_bech32_bytes | uint64 | uint32 | uint32 | address2_bech32_bytes
// into format:
// prefix_bytes | address1_len (1 byte) | address1_bytes | uint64 | uint32 | uint32 | address2_len (1 byte) | address2_bytes
func migratePrefixBech32Uint64Uint32Uint32Bech32(store sdk.KVStore, prefixBz []byte) {
	oldStore := prefix.NewStore(store, prefixBz)

	oldStoreIter := oldStore.Iterator(nil, nil)
	defer oldStoreIter.Close()

	for ; oldStoreIter.Valid(); oldStoreIter.Next() {
		bech32Addr1 := string(oldStoreIter.Key()[:v015.V014Bech32AddrLen])
		addr1, err := sdk.AccAddressFromBech32(bech32Addr1)
		if err != nil {
			panic(err)
		}

		midBz := oldStoreIter.Key()[v015.V014Bech32AddrLen : v015.V014Bech32AddrLen+16]

		bech32Addr2 := string(oldStoreIter.Key()[v015.V014Bech32AddrLen+16:])
		addr2, err := sdk.AccAddressFromBech32(bech32Addr2)
		if err != nil {
			panic(err)
		}

		newStoreKey := append(append(append(prefixBz, address.MustLengthPrefix(addr1)...), midBz...), address.MustLengthPrefix(addr2)...)

		// Set new key on store. Values don't change.
		store.Set(newStoreKey, oldStoreIter.Value())
		oldStore.Delete(oldStoreIter.Key())
	}
}

func migrateLease(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObj v1beta1.Lease
	cdc.MustUnmarshal(oldValueBz, &oldObj)
	return &types.Lease{
		LeaseID: types.LeaseID{
			Owner:    oldObj.LeaseID.Owner,
			DSeq:     oldObj.LeaseID.DSeq,
			GSeq:     oldObj.LeaseID.GSeq,
			OSeq:     oldObj.LeaseID.OSeq,
			Provider: oldObj.LeaseID.Provider,
		},
		State:     types.Lease_State(oldObj.State),
		Price:     sdk.NewDecCoinFromCoin(oldObj.Price),
		CreatedAt: oldObj.CreatedAt,
	}
}

func migrateBid(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject v1beta1.Bid
	cdc.MustUnmarshal(oldValueBz, &oldObject)

	return &types.Bid{
		BidID: types.BidID{
			Owner:    oldObject.BidID.Owner,
			DSeq:     oldObject.BidID.DSeq,
			GSeq:     oldObject.BidID.GSeq,
			OSeq:     oldObject.BidID.OSeq,
			Provider: oldObject.BidID.Provider,
		},
		State:     types.Bid_State(oldObject.State),
		Price:     sdk.NewDecCoinFromCoin(oldObject.Price),
		CreatedAt: oldObject.CreatedAt,
	}
}

func migrateOrder(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject v1beta1.Order
	cdc.MustUnmarshal(oldValueBz, &oldObject)

	return &types.Order{
		OrderID: types.OrderID{
			Owner: oldObject.OrderID.Owner,
			DSeq:  oldObject.OrderID.DSeq,
			GSeq:  oldObject.OrderID.GSeq,
			OSeq:  oldObject.OrderID.OSeq,
		},
		State:     types.Order_State(oldObject.State),
		Spec:      dmigrate.GroupSpecFromV1Beta1(oldObject.Spec),
		CreatedAt: oldObject.CreatedAt,
	}
}

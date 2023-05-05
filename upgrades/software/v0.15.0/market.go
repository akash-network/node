// Package v0_15_0
// nolint revive
package v0_15_0

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	dmigrate "github.com/akash-network/akash-api/go/node/deployment/v1beta2/migrate"
	mv1beta1 "github.com/akash-network/akash-api/go/node/market/v1beta1"
	mv1beta2 "github.com/akash-network/akash-api/go/node/market/v1beta2"

	utypes "github.com/akash-network/node/upgrades/types"
	keys "github.com/akash-network/node/x/market/keeper/keys/v1beta2"
)

type marketMigrations struct {
	utypes.Migrator
}

func newMarketMigration(m utypes.Migrator) utypes.Migration {
	return marketMigrations{Migrator: m}
}

func (m marketMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates market from version 1 to 2.
func (m marketMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	// Change addresses to be length-prefixed
	migratePrefixBech32AddrBytes(store, mv1beta1.OrderPrefix())
	err := migratePrefixBech32Uint64Uint32Uint32Bech32(store, mv1beta1.BidPrefix())
	if err != nil {
		return err
	}

	err = migratePrefixBech32Uint64Uint32Uint32Bech32(store, mv1beta1.LeasePrefix())
	if err != nil {
		return err
	}

	// Migrate protobuf
	err = utypes.MigrateValue(store, m.Codec(), mv1beta1.OrderPrefix(), migrateOrder)
	if err != nil {
		return err
	}

	err = utypes.MigrateValue(store, m.Codec(), mv1beta1.BidPrefix(), migrateBid)
	if err != nil {
		return err
	}

	err = utypes.MigrateValue(store, m.Codec(), mv1beta1.LeasePrefix(), migrateLease)
	if err != nil {
		return err
	}

	// add the mapping of secondary lease key -> lease key
	addSecondaryLeaseKeys(store, m.Codec())

	return nil
}

func addSecondaryLeaseKeys(baseStore sdk.KVStore, cdc codec.BinaryCodec) {
	store := prefix.NewStore(baseStore, mv1beta2.LeasePrefix())

	iter := sdk.KVStorePrefixIterator(store, mv1beta2.LeasePrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var from mv1beta2.Lease
		cdc.MustUnmarshal(iter.Value(), &from)

		leaseKey := keys.LeaseKey(from.GetLeaseID())
		for _, secondaryKey := range keys.SecondaryKeysForLease(from.GetLeaseID()) {
			store.Set(secondaryKey, leaseKey)
		}
	}
}

// migratePrefixBech32Uint64Uint32Uint32Bech32 is a helper function that migrates all keys of format:
// prefix_bytes | address1_bech32_bytes | uint64 | uint32 | uint32 | address2_bech32_bytes
// into format:
// prefix_bytes | address1_len (1 byte) | address1_bytes | uint64 | uint32 | uint32 | address2_len (1 byte) | address2_bytes
func migratePrefixBech32Uint64Uint32Uint32Bech32(store sdk.KVStore, prefixBz []byte) error {
	oldStore := prefix.NewStore(store, prefixBz)

	oldStoreIter := oldStore.Iterator(nil, nil)
	defer func() {
		_ = oldStoreIter.Close()
	}()

	for ; oldStoreIter.Valid(); oldStoreIter.Next() {
		bech32Addr1 := string(oldStoreIter.Key()[:V014Bech32AddrLen])
		addr1, err := sdk.AccAddressFromBech32(bech32Addr1)
		if err != nil {
			return err
		}

		midBz := oldStoreIter.Key()[V014Bech32AddrLen : V014Bech32AddrLen+16]

		bech32Addr2 := string(oldStoreIter.Key()[V014Bech32AddrLen+16:])
		addr2, err := sdk.AccAddressFromBech32(bech32Addr2)
		if err != nil {
			return err
		}

		newStoreKey := append(append(append(prefixBz, address.MustLengthPrefix(addr1)...), midBz...), address.MustLengthPrefix(addr2)...)

		// Set new key on store. Values don't change.
		store.Set(newStoreKey, oldStoreIter.Value())
		oldStore.Delete(oldStoreIter.Key())
	}

	return nil
}

func migrateLease(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObj mv1beta1.Lease
	cdc.MustUnmarshal(fromBz, &oldObj)
	to := mv1beta2.Lease{
		LeaseID: mv1beta2.LeaseID{
			Owner:    oldObj.LeaseID.Owner,
			DSeq:     oldObj.LeaseID.DSeq,
			GSeq:     oldObj.LeaseID.GSeq,
			OSeq:     oldObj.LeaseID.OSeq,
			Provider: oldObj.LeaseID.Provider,
		},
		State:     mv1beta2.Lease_State(oldObj.State),
		Price:     sdk.NewDecCoinFromCoin(oldObj.Price),
		CreatedAt: oldObj.CreatedAt,
		ClosedOn:  0, // For leases created prior to this change never report the data
	}

	return &to
}

func migrateBid(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject mv1beta1.Bid
	cdc.MustUnmarshal(fromBz, &oldObject)

	to := mv1beta2.Bid{
		BidID: mv1beta2.BidID{
			Owner:    oldObject.BidID.Owner,
			DSeq:     oldObject.BidID.DSeq,
			GSeq:     oldObject.BidID.GSeq,
			OSeq:     oldObject.BidID.OSeq,
			Provider: oldObject.BidID.Provider,
		},
		State:     mv1beta2.Bid_State(oldObject.State),
		Price:     sdk.NewDecCoinFromCoin(oldObject.Price),
		CreatedAt: oldObject.CreatedAt,
	}

	return &to
}

func migrateOrder(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject mv1beta1.Order
	cdc.MustUnmarshal(fromBz, &oldObject)

	to := mv1beta2.Order{
		OrderID: mv1beta2.OrderID{
			Owner: oldObject.OrderID.Owner,
			DSeq:  oldObject.OrderID.DSeq,
			GSeq:  oldObject.OrderID.GSeq,
			OSeq:  oldObject.OrderID.OSeq,
		},
		State:     mv1beta2.Order_State(oldObject.State),
		Spec:      dmigrate.GroupSpecFromV1Beta1(oldObject.Spec),
		CreatedAt: oldObject.CreatedAt,
	}

	return &to
}

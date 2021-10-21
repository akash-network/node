package v015

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
)

const (
	V014Bech32AddrLen = 44 // length of an akash address when encoded as a bech32 string in v0.14
)

// MigratePrefixBech32AddrBytes is a helper function that migrates all keys of format:
// prefix_bytes | address_bech32_bytes | arbitrary_bytes
// into format:
// prefix_bytes | address_len (1 byte) | address_bytes | arbitrary_bytes
func MigratePrefixBech32AddrBytes(store sdk.KVStore, prefixBz []byte) {
	oldStore := prefix.NewStore(store, prefixBz)

	oldStoreIter := oldStore.Iterator(nil, nil)
	defer oldStoreIter.Close()

	for ; oldStoreIter.Valid(); oldStoreIter.Next() {
		bech32Addr := string(oldStoreIter.Key()[:V014Bech32AddrLen])
		addr, err := sdk.AccAddressFromBech32(bech32Addr)
		if err != nil {
			panic(err)
		}

		endBz := oldStoreIter.Key()[V014Bech32AddrLen:]
		newStoreKey := append(append(prefixBz, address.MustLengthPrefix(addr)...), endBz...)

		// Set new key on store. Values don't change.
		store.Set(newStoreKey, oldStoreIter.Value())
		oldStore.Delete(oldStoreIter.Key())
	}
}

// ValueMigrator migrates a value to the new protobuf type given its old protobuf serialized bytes.
type ValueMigrator func(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler

// MigrateValue is a helper function that migrates values stored in a KV store for the given
// key prefix using the given value migrator function.
func MigrateValue(store sdk.KVStore, cdc codec.BinaryCodec, prefixBz []byte, migrator ValueMigrator) {
	pStore := prefix.NewStore(store, prefixBz)

	iter := pStore.Iterator(nil, nil)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		pStore.Set(iter.Key(), cdc.MustMarshal(migrator(iter.Value(), cdc)))
	}
}

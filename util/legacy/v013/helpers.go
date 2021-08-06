package v013

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
)

const (
	V012Bech32AddrLen = 44 // length of an akash address when encoded as a bech32 string in v0.12
)

// MigratePrefixBech32AddrBytes is a helper function that migrates all keys of format:
// prefix_bytes | address_bech32_bytes | arbitrary_bytes
// into format:
// prefix_bytes | address_len (1 byte) | address_bytes | arbitrary_bytes
func MigratePrefixBech32AddrBytes(store types.KVStore, prefixBz []byte) {
	oldStore := prefix.NewStore(store, prefixBz)

	oldStoreIter := oldStore.Iterator(nil, nil)
	defer oldStoreIter.Close()

	for ; oldStoreIter.Valid(); oldStoreIter.Next() {
		bech32Addr := string(oldStoreIter.Key()[:V012Bech32AddrLen])
		addr, err := types.AccAddressFromBech32(bech32Addr)
		if err != nil {
			panic(err)
		}

		endBz := oldStoreIter.Key()[V012Bech32AddrLen:]
		newStoreKey := append(append(prefixBz, address.MustLengthPrefix(addr)...), endBz...)

		// Set new key on store. Values don't change.
		store.Set(newStoreKey, oldStoreIter.Value())
		oldStore.Delete(oldStoreIter.Key())
	}
}

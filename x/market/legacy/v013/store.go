package v013

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	v013 "github.com/ovrclk/akash/util/legacy/v013"
	"github.com/ovrclk/akash/x/market/types"
)

// MigrateStore performs in-place store migrations from v0.12 to v0.13. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := ctx.KVStore(storeKey)
	v013.MigratePrefixBech32AddrBytes(store, types.OrderPrefix)
	migratePrefixBech32Uint64Uint32Uint32Bech32(store, types.BidPrefix)
	migratePrefixBech32Uint64Uint32Uint32Bech32(store, types.LeasePrefix)

	return nil
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
		bech32Addr1 := string(oldStoreIter.Key()[:v013.V012Bech32AddrLen])
		addr1, err := sdk.AccAddressFromBech32(bech32Addr1)
		if err != nil {
			panic(err)
		}

		midBz := oldStoreIter.Key()[v013.V012Bech32AddrLen : v013.V012Bech32AddrLen+16]

		bech32Addr2 := string(oldStoreIter.Key()[v013.V012Bech32AddrLen+16:])
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

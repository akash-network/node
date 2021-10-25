package v015

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
)

// MigrateStore performs in-place store migrations from v0.14 to v0.15. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := ctx.KVStore(storeKey)
	migrateProviderKeys(store)

	return nil
}

// migrateProviderKeys migrate the provider keys to cater for variable-length
// addresses.
func migrateProviderKeys(store sdk.KVStore) {
	// old key is of format:
	// ownerAddrBytes (20 bytes)
	// new key is of format
	// ownerAddrLen (1 byte) || ownerAddrBytes

	oldStoreIter := store.Iterator(nil, nil)
	defer oldStoreIter.Close()

	for ; oldStoreIter.Valid(); oldStoreIter.Next() {
		newStoreKey := address.MustLengthPrefix(oldStoreIter.Key())

		// Set new key on store. Values don't change.
		store.Set(newStoreKey, oldStoreIter.Value())
		store.Delete(oldStoreIter.Key())
	}
}

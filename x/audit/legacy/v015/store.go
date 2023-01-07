package v015

import (
	types "github.com/akash-network/node/x/audit/types/v1beta2"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v043 "github.com/cosmos/cosmos-sdk/x/distribution/legacy/v043"
)

// MigrateStore performs in-place store migrations from v0.14 to v0.15. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := ctx.KVStore(storeKey)
	v043.MigratePrefixAddressAddress(store, types.PrefixProviderID())

	return nil
}

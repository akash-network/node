package v013

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	v043 "github.com/cosmos/cosmos-sdk/x/distribution/legacy/v043"
	"github.com/ovrclk/akash/x/audit/types"
)

// MigrateStore performs in-place store migrations from v0.12 to v0.13. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := ctx.KVStore(storeKey)
	v043.MigratePrefixAddressAddress(store, types.PrefixProviderID())

	return nil
}

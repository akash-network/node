package v015

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	v043 "github.com/cosmos/cosmos-sdk/x/distribution/legacy/v043"
	types "github.com/ovrclk/akash/x/cert/types/v1beta2"
)

// MigrateStore performs in-place store migrations from v0.14 to v0.15. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := ctx.KVStore(storeKey)
	v043.MigratePrefixAddressBytes(store, types.PrefixCertificateID())

	return nil
}

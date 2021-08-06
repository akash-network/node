package v013

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	v013 "github.com/ovrclk/akash/util/legacy/v013"
	"github.com/ovrclk/akash/x/deployment/types"
)

// MigrateStore performs in-place store migrations from v0.12 to v0.13. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := ctx.KVStore(storeKey)
	v013.MigratePrefixBech32AddrBytes(store, types.DeploymentPrefix)
	v013.MigratePrefixBech32AddrBytes(store, types.GroupPrefix)

	return nil
}

package v015

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/keeper/keys"

	types "github.com/ovrclk/akash/x/market/types/v1beta2"
)

// MigrateStore performs in-place store migrations from v0.13 to v0.14. The
// migration includes:
//
// - Change deployments storage from single value to an array
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := prefix.NewStore(ctx.KVStore(storeKey), types.LeasePrefix())

	iter := sdk.KVStorePrefixIterator(store, types.LeasePrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var lease types.Lease

		if err := types.ModuleCdc.Unmarshal(iter.Value(), &lease); err != nil {
			return err
		}

		leaseKey := keys.LeaseKey(lease.GetLeaseID())
		for _, secondaryKey := range keys.SecondaryKeysForLease(lease.GetLeaseID()) {
			store.Set(secondaryKey, leaseKey)
		}
	}

	return nil
}

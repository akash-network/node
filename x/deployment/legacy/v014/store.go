package v014

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/deployment/types/v1beta1"
	types "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	dmigrate "github.com/ovrclk/akash/x/deployment/types/v1beta2/migrate"
)

// MigrateStore performs in-place store migrations from v0.13 to v0.14. The
// migration includes:
//
// - Change deployments storage from single value to an array
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := prefix.NewStore(ctx.KVStore(storeKey), types.GroupPrefix())

	iter := sdk.KVStorePrefixIterator(store, types.DeploymentPrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var oval v1beta1.Group

		if err := types.ModuleCdc.Unmarshal(iter.Value(), &oval); err != nil {
			return err
		}

		val := dmigrate.GroupFromV1Beta1(oval)

		nval, err := types.ModuleCdc.Marshal(&val)
		if err != nil {
			return err
		}

		store.Set(iter.Key(), nval)
	}

	return nil
}

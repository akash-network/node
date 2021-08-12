package v014

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	akashtypes "github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/x/deployment/types"
	ldtypes "github.com/ovrclk/akash/x/deployment/types/legacy"
)

// MigrateStore performs in-place store migrations from v0.12 to v0.13. The
// migration includes:
//
// - Change addresses to be length-prefixed
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := prefix.NewStore(ctx.KVStore(storeKey), types.GroupPrefix())

	iter := sdk.KVStorePrefixIterator(store, types.DeploymentPrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var lval ldtypes.Group

		if err := types.ModuleCdc.Unmarshal(iter.Value(), &lval); err != nil {
			return err
		}

		val := types.Group{
			GroupID: lval.GroupID,
			State:   types.Group_State(lval.State),
			GroupSpec: types.GroupSpec{
				Name:         lval.GroupSpec.Name,
				Requirements: lval.GroupSpec.Requirements,
			},
			CreatedAt: lval.CreatedAt,
		}

		for _, res := range lval.GroupSpec.Resources {
			runits := akashtypes.ResourceUnits{
				CPU:       res.Resources.CPU,
				Memory:    res.Resources.Memory,
				Storage:   akashtypes.Volumes{},
				Endpoints: res.Resources.Endpoints,
			}

			if storage := res.Resources.Storage; storage != nil {
				runits.Storage = append(runits.Storage, *storage)
			}

			val.GroupSpec.Resources = append(val.GroupSpec.Resources, types.Resource{
				Resources: akashtypes.ResourceUnits{},
				Count:     res.Count,
				Price:     res.Price,
			})
		}

		nval, err := types.ModuleCdc.Marshal(&val)
		if err != nil {
			return err
		}

		store.Set(iter.Key(), nval)
	}

	return nil
}

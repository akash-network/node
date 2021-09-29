package v014

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/types"
	ldtypes "github.com/ovrclk/akash/x/market/types/legacy"
)

// MigrateStore performs in-place store migrations from v0.13 to v0.16. The
// migration includes:
// Upgrade the lease type to have closed_on
//

func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey) error {
	store := prefix.NewStore(ctx.KVStore(storeKey), types.LeasePrefix())

	iter := sdk.KVStorePrefixIterator(store, types.LeasePrefix())
	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var lval ldtypes.Lease

		if err := types.ModuleCdc.Unmarshal(iter.Value(), &lval); err != nil {
			return err
		}

		val := types.Lease{
			LeaseID: types.LeaseID{
				Owner:    lval.LeaseID.Owner,
				DSeq:     lval.LeaseID.DSeq,
				GSeq:     lval.LeaseID.GSeq,
				OSeq:     lval.LeaseID.OSeq,
				Provider: lval.LeaseID.Provider,
			},
			State:     types.Lease_State(lval.State),
			Price:     lval.Price,
			CreatedAt: lval.CreatedAt,
			ClosedOn:  0, // default value of 0
		}

		nval, err := types.ModuleCdc.Marshal(&val)
		if err != nil {
			return err
		}

		store.Set(iter.Key(), nval)
	}

	return nil
}

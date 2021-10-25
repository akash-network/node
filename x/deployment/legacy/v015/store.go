package v015

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v015 "github.com/ovrclk/akash/util/legacy/v015"
	"github.com/ovrclk/akash/x/deployment/types/v1beta1"
	types "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	"github.com/ovrclk/akash/x/deployment/types/v1beta2/migrate"
)

// MigrateStore performs in-place store migrations from v0.14 to v0.15. The
// migration includes:
//
// - Change addresses to be length-prefixed
// - Migrating Group proto from v1beta1 to v1beta2
// 		- Change deployments storage from single value to an array
// 		- Change resource price from Coin to DecCoin
func MigrateStore(ctx sdk.Context, storeKey sdk.StoreKey, cdc codec.BinaryCodec) error {
	store := ctx.KVStore(storeKey)

	v015.MigratePrefixBech32AddrBytes(store, types.DeploymentPrefix())
	v015.MigratePrefixBech32AddrBytes(store, types.GroupPrefix())

	v015.MigrateValue(store, cdc, types.GroupPrefix(), migrateGroup)

	return nil
}

func migrateGroup(oldGroupBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject v1beta1.Group
	cdc.MustUnmarshal(oldGroupBz, &oldObject)

	newObj := migrate.GroupFromV1Beta1(oldObject)
	return &newObj
}

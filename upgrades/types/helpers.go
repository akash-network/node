//nolint: revive

package types

import (
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/gogoproto/proto"
)

// ValueMigrator migrates a value to the new protobuf type given its old protobuf serialized bytes.
type ValueMigrator func(fromBz []byte, cdc codec.BinaryCodec) proto.Message

// MigrateValue is a helper function that migrates values stored in a KV store for the given
// key prefix using the given value migrator function.
func MigrateValue(store storetypes.KVStore, cdc codec.BinaryCodec, prefixBz []byte, pStart, pEnd []byte, migrator ValueMigrator) (err error) {
	pStore := prefix.NewStore(store, prefixBz)

	iter := pStore.Iterator(pStart, pEnd)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		nVal := migrator(iter.Value(), cdc)
		bz := cdc.MustMarshal(nVal)

		pStore.Set(iter.Key(), bz)
	}

	return nil
}

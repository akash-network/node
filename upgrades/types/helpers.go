package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ValueMigrator migrates a value to the new protobuf type given its old protobuf serialized bytes.
type ValueMigrator func(oldValueBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler

// MigrateValue is a helper function that migrates values stored in a KV store for the given
// key prefix using the given value migrator function.
func MigrateValue(store sdk.KVStore, cdc codec.BinaryCodec, prefixBz []byte, migrator ValueMigrator) (err error) {
	pStore := prefix.NewStore(store, prefixBz)

	iter := pStore.Iterator(nil, nil)
	defer func() {
		err = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		pStore.Set(iter.Key(), cdc.MustMarshal(migrator(iter.Value(), cdc)))
	}

	return nil
}

// Package v0_24_0
// nolint revive
package v0_24_0

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	dmigrate "github.com/akash-network/akash-api/go/node/deployment/v1beta3/migrate"
	mv1beta2 "github.com/akash-network/akash-api/go/node/market/v1beta2"
	mv1beta3 "github.com/akash-network/akash-api/go/node/market/v1beta3"

	utypes "github.com/akash-network/node/upgrades/types"
)

type marketMigrations struct {
	utypes.Migrator
}

func newMarketMigration(m utypes.Migrator) utypes.Migration {
	return marketMigrations{Migrator: m}
}

func (m marketMigrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates deployment from version 2 to 3.
func (m marketMigrations) handler(ctx sdk.Context) error {
	store := ctx.KVStore(m.StoreKey())

	err := utypes.MigrateValue(store, m.Codec(), mv1beta3.OrderPrefix(), migrateOrder)

	if err != nil {
		return err
	}

	return nil
}

func migrateOrder(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject mv1beta2.Order
	cdc.MustUnmarshal(fromBz, &oldObject)

	to := mv1beta3.Order{
		OrderID: mv1beta3.OrderID{
			Owner: oldObject.OrderID.Owner,
			DSeq:  oldObject.OrderID.DSeq,
			GSeq:  oldObject.OrderID.GSeq,
			OSeq:  oldObject.OrderID.OSeq,
		},
		State:     mv1beta3.Order_State(oldObject.State),
		Spec:      dmigrate.GroupSpecFromV1Beta2(oldObject.Spec),
		CreatedAt: oldObject.CreatedAt,
	}

	return &to
}

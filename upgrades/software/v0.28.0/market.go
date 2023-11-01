// Package v0_28_0
// nolint revive
package v0_28_0

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	mv1beta3 "github.com/akash-network/akash-api/go/node/market/v1beta3"
	mmigrate "github.com/akash-network/akash-api/go/node/market/v1beta4/migrate"

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

	err := utypes.MigrateValue(store, m.Codec(), mv1beta3.BidPrefix(), migrateBid)

	if err != nil {
		return err
	}

	return nil
}

func migrateBid(fromBz []byte, cdc codec.BinaryCodec) codec.ProtoMarshaler {
	var oldObject mv1beta3.Bid
	cdc.MustUnmarshal(fromBz, &oldObject)

	to := mmigrate.BidFromV1beta3(oldObject)

	return &to
}

package v3_0_0

import (
	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkmodule "github.com/cosmos/cosmos-sdk/types/module"

	mv1 "pkg.akt.dev/go/node/market/v1"

	utypes "pkg.akt.dev/node/v3/upgrades/types"
	marketkeeper "pkg.akt.dev/node/v3/x/market/keeper"
	"pkg.akt.dev/node/v3/x/market/keeper/keys"
)

type marketV9Migrations struct {
	utypes.Migrator
}

func newMarketV9Migration(m utypes.Migrator) utypes.Migration {
	return marketV9Migrations{Migrator: m}
}

func (m marketV9Migrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates market from version 8 to 9.
func (m marketV9Migrations) handler(_ sdk.Context) error {
	return nil
}

type marketV10Migrations struct {
	utypes.Migrator
}

func newMarketV10Migration(m utypes.Migrator) utypes.Migration {
	return marketV10Migrations{Migrator: m}
}

func (m marketV10Migrations) GetHandler() sdkmodule.MigrationHandler {
	return m.handler
}

// handler migrates market from version 9 to 10.
func (m marketV10Migrations) handler(ctx sdk.Context) error {
	skey := m.StoreKey().(*storetypes.KVStoreKey)
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	leaseIndexes := marketkeeper.NewLeaseIndexes(sb)
	leases := collections.NewIndexedMap(sb, collections.NewPrefix(keys.LeasePrefixNew), "leases", keys.LeasePrimaryKeyCodec, codec.CollValue[mv1.Lease](m.Codec()), leaseIndexes)
	leaseStats := marketkeeper.NewProviderLeaseStatsMap(sb)

	if _, err := sb.Build(); err != nil {
		return err
	}

	return marketkeeper.BackfillProviderLeaseStats(ctx, leases, leaseStats)
}

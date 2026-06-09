package v3_0_0

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	"cosmossdk.io/store"
	storemetrics "cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	sdk "github.com/cosmos/cosmos-sdk/types"
	testutilmod "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/provider/v1beta4"
	"pkg.akt.dev/go/testutil"

	utypes "pkg.akt.dev/node/v3/upgrades/types"
	"pkg.akt.dev/node/v3/x/provider/keeper"
)

func TestProviderMigrationBackfillsRegistrationsAtUpgradeBlockTime(t *testing.T) {
	ctx, kpr := setupProviderMigrationKeeper(t)
	ctx = ctx.WithBlockTime(time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	store := ctx.KVStore(kpr.StoreKey())

	prov := testutil.Provider(t)
	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)
	store.Set(keeper.ProviderKey(owner), kpr.Codec().MustMarshal(&prov))

	migration := newProviderMigration(utypes.NewMigrator(kpr.Codec(), kpr.StoreKey()))
	require.NoError(t, migration.GetHandler()(ctx))

	registration, found := kpr.GetRegistration(ctx, owner)
	require.True(t, found)
	require.Equal(t, types.ProviderRegistration{
		Owner:        prov.Owner,
		RegisteredAt: ctx.BlockTime(),
	}, registration)
}

func TestProviderMigrationKeepsExistingRegistrations(t *testing.T) {
	ctx, kpr := setupProviderMigrationKeeper(t)
	ctx = ctx.WithBlockTime(time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	store := ctx.KVStore(kpr.StoreKey())

	prov := testutil.Provider(t)
	owner, err := sdk.AccAddressFromBech32(prov.Owner)
	require.NoError(t, err)
	store.Set(keeper.ProviderKey(owner), kpr.Codec().MustMarshal(&prov))

	existing := types.ProviderRegistration{
		Owner:        prov.Owner,
		RegisteredAt: ctx.BlockTime().Add(-24 * time.Hour),
	}
	require.NoError(t, kpr.SetRegistration(ctx, existing))

	migration := newProviderMigration(utypes.NewMigrator(kpr.Codec(), kpr.StoreKey()))
	require.NoError(t, migration.GetHandler()(ctx))

	registration, found := kpr.GetRegistration(ctx, owner)
	require.True(t, found)
	require.Equal(t, existing, registration)
}

func TestProviderMigrationBackfillsMissingRegistrationsOnly(t *testing.T) {
	ctx, kpr := setupProviderMigrationKeeper(t)
	ctx = ctx.WithBlockTime(time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC))
	store := ctx.KVStore(kpr.StoreKey())

	prov1 := testutil.Provider(t)
	prov2 := testutil.Provider(t)
	owner1, err := sdk.AccAddressFromBech32(prov1.Owner)
	require.NoError(t, err)
	owner2, err := sdk.AccAddressFromBech32(prov2.Owner)
	require.NoError(t, err)
	store.Set(keeper.ProviderKey(owner1), kpr.Codec().MustMarshal(&prov1))
	store.Set(keeper.ProviderKey(owner2), kpr.Codec().MustMarshal(&prov2))

	existing := types.ProviderRegistration{
		Owner:        prov1.Owner,
		RegisteredAt: ctx.BlockTime().Add(-24 * time.Hour),
	}
	require.NoError(t, kpr.SetRegistration(ctx, existing))

	migration := newProviderMigration(utypes.NewMigrator(kpr.Codec(), kpr.StoreKey()))
	require.NoError(t, migration.GetHandler()(ctx))

	registration1, found := kpr.GetRegistration(ctx, owner1)
	require.True(t, found)
	require.Equal(t, existing, registration1)

	registration2, found := kpr.GetRegistration(ctx, owner2)
	require.True(t, found)
	require.Equal(t, types.ProviderRegistration{
		Owner:        prov2.Owner,
		RegisteredAt: ctx.BlockTime(),
	}, registration2)
}

func setupProviderMigrationKeeper(t testing.TB) (sdk.Context, keeper.IKeeper) {
	t.Helper()

	cfg := testutilmod.MakeTestEncodingConfig()
	key := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()

	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)

	require.NoError(t, ms.LoadLatestVersion())

	ctx := sdk.NewContext(ms, tmproto.Header{}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(cfg.Codec, key, "")
}

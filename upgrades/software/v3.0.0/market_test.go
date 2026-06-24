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

	mv1 "pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/testutil"

	utypes "pkg.akt.dev/node/v3/upgrades/types"
	"pkg.akt.dev/node/v3/x/market/keeper"
)

func TestMarketMigrationBackfillsProviderLeaseStats(t *testing.T) {
	ctx, kpr := setupMarketMigrationKeeper(t)
	provider := testutil.AccAddress(t)

	saveMarketMigrationLease(t, ctx, kpr, provider, mv1.LeaseClosed, mv1.LeaseClosedReasonOwner)
	saveMarketMigrationLease(t, ctx, kpr, provider, mv1.LeaseClosed, mv1.LeaseClosedReasonProvider)
	saveMarketMigrationLease(t, ctx, kpr, provider, mv1.LeaseActive, mv1.LeaseClosedReasonUnstable)
	saveMarketMigrationLease(t, ctx, kpr, testutil.AccAddress(t), mv1.LeaseClosed, mv1.LeaseClosedReasonProvider)

	migration := newMarketV10Migration(utypes.NewMigrator(kpr.Codec(), kpr.StoreKey()))
	require.NoError(t, migration.GetHandler()(ctx))

	completed, failures, found := kpr.GetProviderLeaseStats(ctx, provider, time.Time{})
	require.True(t, found)
	require.Equal(t, uint64(1), completed)
	require.Equal(t, map[mv1.LeaseClosedReason]uint64{
		mv1.LeaseClosedReasonProvider: 1,
	}, failures)
}

func saveMarketMigrationLease(
	t testing.TB,
	ctx sdk.Context,
	kpr keeper.IKeeper,
	provider sdk.AccAddress,
	state mv1.Lease_State,
	reason mv1.LeaseClosedReason,
) {
	t.Helper()

	require.NoError(t, kpr.SaveLease(ctx, mv1.Lease{
		ID:     testutil.LeaseIDForAccount(t, testutil.AccAddress(t), provider),
		State:  state,
		Reason: reason,
	}))
}

func setupMarketMigrationKeeper(t testing.TB) (sdk.Context, keeper.IKeeper) {
	t.Helper()

	cfg := testutilmod.MakeTestEncodingConfig()
	key := storetypes.NewKVStoreKey(mv1.StoreKey)
	db := dbm.NewMemDB()

	ms := store.NewCommitMultiStore(db, log.NewNopLogger(), storemetrics.NewNoOpMetrics())
	ms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)

	require.NoError(t, ms.LoadLatestVersion())

	ctx := sdk.NewContext(ms, tmproto.Header{}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(cfg.Codec, key, nil, "")
}

package market

import (
	"testing"

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
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/x/market/keeper"
)

func TestInitGenesisBackfillsProviderLeaseStats(t *testing.T) {
	ctx, kpr := setupGenesisMarketKeeper(t)
	provider := testutil.AccAddress(t)

	ownerClosed := testGenesisLease(t, provider, mv1.LeaseClosed, mv1.LeaseClosedReasonOwner)
	providerFailed := testGenesisLease(t, provider, mv1.LeaseClosed, mv1.LeaseClosedReasonUnstable)
	active := testGenesisLease(t, provider, mv1.LeaseActive, mv1.LeaseClosedReasonProvider)

	InitGenesis(ctx, kpr, &mvbeta.GenesisState{
		Params: mvbeta.DefaultParams(),
		Leases: mv1.Leases{ownerClosed, providerFailed, active},
	})

	completed, failures, found := kpr.GetProviderLeaseStats(ctx, provider)
	require.True(t, found)
	require.Equal(t, uint64(1), completed)
	require.Equal(t, map[mv1.LeaseClosedReason]uint64{
		mv1.LeaseClosedReasonUnstable: 1,
	}, failures)
}

func testGenesisLease(t testing.TB, provider sdk.AccAddress, state mv1.Lease_State, reason mv1.LeaseClosedReason) mv1.Lease {
	t.Helper()

	return mv1.Lease{
		ID:     testutil.LeaseIDForAccount(t, testutil.AccAddress(t), provider),
		State:  state,
		Reason: reason,
	}
}

func setupGenesisMarketKeeper(t testing.TB) (sdk.Context, keeper.IKeeper) {
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

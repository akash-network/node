package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	"pkg.akt.dev/go/testutil"
)

func Test_CreateOrderWithReclamation(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)
	reclamation := &dv1.DeploymentReclamation{
		MinWindow: 24 * time.Hour,
	}

	order, err := keeper.CreateOrder(ctx, group.ID, group.GroupSpec, reclamation)
	require.NoError(t, err)
	require.NotNil(t, order.Reclamation)
	assert.Equal(t, 24*time.Hour, order.Reclamation.MinWindow)
}

func Test_CreateOrderWithoutReclamation(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)

	order, err := keeper.CreateOrder(ctx, group.ID, group.GroupSpec, nil)
	require.NoError(t, err)
	assert.Nil(t, order.Reclamation)
}

func Test_CreateBidWithReclamationWindow(t *testing.T) {
	_, _, suite := setupKeeper(t)
	ctx := suite.Context()
	keeper := suite.MarketKeeper()

	order, _ := createOrder(t, ctx, keeper)
	provider := testutil.AccAddress(t)
	price := testutil.ACTDecCoinRandom(t)
	roffer := mvbeta.ResourceOfferFromRU(order.Spec.Resources)
	bidID := mv1.MakeBidID(order.ID, provider)

	window := 48 * time.Hour
	bid, err := keeper.CreateBid(ctx, bidID, price, roffer, &window)
	require.NoError(t, err)
	require.NotNil(t, bid.ReclamationWindow)
	assert.Equal(t, 48*time.Hour, *bid.ReclamationWindow)
}

func Test_CreateBidWithoutReclamationWindow(t *testing.T) {
	_, _, suite := setupKeeper(t)
	ctx := suite.Context()
	keeper := suite.MarketKeeper()

	order, _ := createOrder(t, ctx, keeper)
	provider := testutil.AccAddress(t)
	price := testutil.ACTDecCoinRandom(t)
	roffer := mvbeta.ResourceOfferFromRU(order.Spec.Resources)
	bidID := mv1.MakeBidID(order.ID, provider)

	bid, err := keeper.CreateBid(ctx, bidID, price, roffer, nil)
	require.NoError(t, err)
	assert.Nil(t, bid.ReclamationWindow)
}

func Test_LeaseReclamationStoredFromBid(t *testing.T) {
	_, _, suite := setupKeeper(t)
	ctx := suite.Context()
	keeper := suite.MarketKeeper()

	// Create order and bid with reclamation window
	order, _ := createOrder(t, ctx, keeper)
	provider := testutil.AccAddress(t)
	price := testutil.ACTDecCoinRandom(t)
	roffer := mvbeta.ResourceOfferFromRU(order.Spec.Resources)
	bidID := mv1.MakeBidID(order.ID, provider)
	window := 24 * time.Hour

	bid, err := keeper.CreateBid(ctx, bidID, price, roffer, &window)
	require.NoError(t, err)

	// Create lease
	err = keeper.CreateLease(ctx, bid)
	require.NoError(t, err)

	// Get lease and manually set reclamation (simulating what the handler does)
	lease, found := keeper.GetLease(ctx, bid.ID.LeaseID())
	require.True(t, found)
	assert.Equal(t, mv1.LeaseActive, lease.State)

	// Simulate handler storing reclamation on the lease
	lease.Reclamation = &mv1.Reclamation{
		Window: *bid.ReclamationWindow,
	}
	err = keeper.SaveLease(ctx, lease)
	require.NoError(t, err)

	// Verify reclamation is persisted
	lease, found = keeper.GetLease(ctx, bid.ID.LeaseID())
	require.True(t, found)
	require.NotNil(t, lease.Reclamation)
	assert.Equal(t, 24*time.Hour, lease.Reclamation.Window)
	assert.Equal(t, int64(0), lease.Reclamation.StartedAt)
	assert.Equal(t, int64(0), lease.Reclamation.Deadline)
}

func Test_LeaseStartReclaim(t *testing.T) {
	_, _, suite := setupKeeper(t)
	ctx := suite.Context()
	keeper := suite.MarketKeeper()

	// Create order, bid, lease with reclamation
	order, _ := createOrder(t, ctx, keeper)
	provider := testutil.AccAddress(t)
	price := testutil.ACTDecCoinRandom(t)
	roffer := mvbeta.ResourceOfferFromRU(order.Spec.Resources)
	bidID := mv1.MakeBidID(order.ID, provider)
	window := 24 * time.Hour

	bid, err := keeper.CreateBid(ctx, bidID, price, roffer, &window)
	require.NoError(t, err)

	err = keeper.CreateLease(ctx, bid)
	require.NoError(t, err)

	// Store reclamation on lease
	lease, found := keeper.GetLease(ctx, bid.ID.LeaseID())
	require.True(t, found)
	lease.Reclamation = &mv1.Reclamation{Window: window}
	err = keeper.SaveLease(ctx, lease)
	require.NoError(t, err)

	// Set block time and height
	blockTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	suite.SetBlockHeight(100)
	ctx = suite.Context().WithBlockTime(blockTime)

	// Start reclamation
	lease, found = keeper.GetLease(ctx, bid.ID.LeaseID())
	require.True(t, found)
	lease.State = mv1.LeaseReclaiming
	lease.Reclamation.StartedAt = ctx.BlockHeight()
	lease.Reclamation.Deadline = blockTime.Add(window).Unix()
	lease.Reclamation.Reason = mv1.LeaseClosedReason(10001)
	err = keeper.SaveLease(ctx, lease)
	require.NoError(t, err)

	// Verify state
	lease, found = keeper.GetLease(ctx, bid.ID.LeaseID())
	require.True(t, found)
	assert.Equal(t, mv1.LeaseReclaiming, lease.State)
	assert.Equal(t, int64(100), lease.Reclamation.StartedAt)
	assert.Equal(t, blockTime.Add(24*time.Hour).Unix(), lease.Reclamation.Deadline)
	assert.Equal(t, mv1.LeaseClosedReason(10001), lease.Reclamation.Reason)
}

func Test_OnLeaseClosedFromReclaiming(t *testing.T) {
	_, _, suite := setupKeeper(t)

	leaseID := createLease(t, suite)
	keeper := suite.MarketKeeper()
	ctx := suite.Context()

	// Get lease and set it to reclaiming with reclamation data
	lease, found := keeper.GetLease(ctx, leaseID)
	require.True(t, found)
	lease.State = mv1.LeaseReclaiming
	lease.Reclamation = &mv1.Reclamation{
		Window:    24 * time.Hour,
		StartedAt: 50,
		Deadline:  time.Now().Add(24 * time.Hour).Unix(),
		Reason:    mv1.LeaseClosedReason(10001),
	}
	err := keeper.SaveLease(ctx, lease)
	require.NoError(t, err)

	// Close from reclaiming state
	suite.SetBlockHeight(200)
	ctx = suite.Context()

	lease, found = keeper.GetLease(ctx, leaseID)
	require.True(t, found)

	err = keeper.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReason(10001))
	require.NoError(t, err)

	// Verify closed
	lease, found = keeper.GetLease(ctx, leaseID)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseClosed, lease.State)
	assert.Equal(t, int64(200), lease.ClosedOn)
}

func Test_OnLeaseClosedFromReclaimingIdempotent(t *testing.T) {
	_, _, suite := setupKeeper(t)

	leaseID := createLease(t, suite)
	keeper := suite.MarketKeeper()
	ctx := suite.Context()

	// Set to reclaiming then close
	lease, found := keeper.GetLease(ctx, leaseID)
	require.True(t, found)
	lease.State = mv1.LeaseReclaiming
	lease.Reclamation = &mv1.Reclamation{Window: 1 * time.Hour}
	err := keeper.SaveLease(ctx, lease)
	require.NoError(t, err)

	suite.SetBlockHeight(100)
	ctx = suite.Context()
	lease, _ = keeper.GetLease(ctx, leaseID)

	err = keeper.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReason(10001))
	require.NoError(t, err)

	// Close again -- should be idempotent (skipped)
	lease, _ = keeper.GetLease(ctx, leaseID)
	err = keeper.OnLeaseClosed(ctx, lease, mv1.LeaseClosed, mv1.LeaseClosedReason(10001))
	require.NoError(t, err)

	lease, found = keeper.GetLease(ctx, leaseID)
	require.True(t, found)
	assert.Equal(t, mv1.LeaseClosed, lease.State)
}

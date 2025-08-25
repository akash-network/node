package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	"pkg.akt.dev/go/node/market/v1"
	types "pkg.akt.dev/go/node/market/v1beta5"
	aauthz "pkg.akt.dev/go/node/types/authz/v1"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/testutil/state"
	"pkg.akt.dev/node/x/market/keeper"
)

func Test_CreateOrder(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)
	order, gspec := createOrder(t, ctx, keeper)

	// assert one active for group
	_, err := keeper.CreateOrder(ctx, order.ID.GroupID(), gspec)
	require.Error(t, err)
}

func Test_GetOrder(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)
	result, ok := keeper.GetOrder(ctx, order.ID)
	require.True(t, ok)
	require.Equal(t, order, result)

	// assert non-existent order fails
	{
		_, ok := keeper.GetOrder(ctx, testutil.OrderID(t))
		require.False(t, ok)
	}
}

func Test_WithOrders(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	count := 0
	keeper.WithOrders(ctx, func(result types.Order) bool {
		if assert.Equal(t, order.ID, result.ID) {
			count++
		}
		return false
	})

	assert.Equal(t, 1, count)
}

func Test_WithOrdersForGroup(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	// create extra orders
	createOrder(t, ctx, keeper)

	count := 0
	keeper.WithOrdersForGroup(ctx, order.ID.GroupID(), types.OrderOpen, func(result types.Order) bool {
		if assert.Equal(t, order.ID, result.ID) {
			count++
		}
		return false
	})

	assert.Equal(t, 1, count)
}

func Test_CreateBid(t *testing.T) {
	_, _, suite := setupKeeper(t)
	createBid(t, suite)
}

func Test_GetBid(t *testing.T) {
	ctx, _, suite := setupKeeper(t)
	bid, _ := createBid(t, suite)

	keeper := suite.MarketKeeper()

	result, ok := keeper.GetBid(ctx, bid.ID)
	require.True(t, ok)
	assert.Equal(t, bid, result)

	// non-existent
	{
		_, ok := keeper.GetBid(ctx, testutil.BidID(t))
		require.False(t, ok)
	}
}

func Test_WithBids(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	bid, _ := createBid(t, suite)
	count := 0
	keeper.WithBids(ctx, func(result types.Bid) bool {
		if assert.Equal(t, bid.ID, result.ID) {
			count++
		}
		return false
	})
	assert.Equal(t, 1, count)
}

func Test_WithBidsForOrder(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	bid, _ := createBid(t, suite)

	// create extra bids
	createBid(t, suite)
	createBid(t, suite)

	count := 0

	keeper.WithBidsForOrder(ctx, bid.ID.OrderID(), types.BidOpen, func(result types.Bid) bool {
		if assert.Equal(t, bid.ID, result.ID) {
			count++
		}
		return false
	})
	assert.Equal(t, 1, count)
}

func Test_GetLease(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	lease, ok := keeper.GetLease(ctx, id)
	assert.True(t, ok)
	assert.Equal(t, id, lease.ID)

	// non-existent
	{
		_, ok := keeper.GetLease(ctx, testutil.LeaseID(t))
		require.False(t, ok)
	}
}

func Test_WithLeases(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	count := 0
	keeper.WithLeases(ctx, func(result v1.Lease) bool {
		if assert.Equal(t, id, result.ID) {
			count++
		}
		return false
	})
	assert.Equal(t, 1, count)
}

func Test_LeaseForOrder(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	// extra leases
	createLease(t, suite)
	createLease(t, suite)

	result, ok := keeper.LeaseForOrder(ctx, types.BidActive, id.OrderID())
	assert.True(t, ok)

	assert.Equal(t, id, result.ID)

	// no match
	{
		bid, _ := createBid(t, suite)
		_, ok := keeper.LeaseForOrder(ctx, types.BidActive, bid.ID.OrderID())
		assert.False(t, ok)
	}
}

func Test_OnOrderMatched(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	result, ok := keeper.GetOrder(ctx, id.OrderID())
	require.True(t, ok)
	assert.Equal(t, types.OrderActive, result.State)
}

func Test_OnBidMatched(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	result, ok := keeper.GetBid(ctx, id.BidID())
	require.True(t, ok)
	assert.Equal(t, types.BidActive, result.State)
}

func Test_OnBidLost(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	bid, _ := createBid(t, suite)

	keeper.OnBidLost(ctx, bid)
	result, ok := keeper.GetBid(ctx, bid.ID)
	require.True(t, ok)
	assert.Equal(t, types.BidLost, result.State)
}

func Test_OnOrderClosed(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	err := keeper.OnOrderClosed(ctx, order)
	require.NoError(t, err)

	result, ok := keeper.GetOrder(ctx, order.ID)
	require.True(t, ok)
	assert.Equal(t, types.OrderClosed, result.State)
}

func Test_OnLeaseClosed(t *testing.T) {
	_, keeper, suite := setupKeeper(t)
	suite.SetBlockHeight(1)
	id := createLease(t, suite)

	lease, ok := keeper.GetLease(suite.Context(), id)
	require.True(t, ok)
	require.Equal(t, int64(0), lease.ClosedOn)

	const testBlockHeight = 1337
	suite.SetBlockHeight(testBlockHeight)

	require.Equal(t, v1.LeaseActive, lease.State)
	err := keeper.OnLeaseClosed(suite.Context(), lease, v1.LeaseClosed)
	require.NoError(t, err)

	result, ok := keeper.GetLease(suite.Context(), id)
	require.True(t, ok)
	assert.Equal(t, v1.LeaseClosed, result.State)
	assert.Equal(t, int64(testBlockHeight), result.ClosedOn)
}

func Test_OnGroupClosed(t *testing.T) {
	_, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	const testBlockHeight = 133
	suite.SetBlockHeight(testBlockHeight)
	err := keeper.OnGroupClosed(suite.Context(), id.BidID().GroupID())
	require.NoError(t, err)

	lease, ok := keeper.GetLease(suite.Context(), id)
	require.True(t, ok)
	assert.Equal(t, v1.LeaseClosed, lease.State)
	assert.Equal(t, int64(testBlockHeight), lease.ClosedOn)

	bid, ok := keeper.GetBid(suite.Context(), id.BidID())
	require.True(t, ok)
	assert.Equal(t, types.BidClosed, bid.State)

	order, ok := keeper.GetOrder(suite.Context(), id.OrderID())
	require.True(t, ok)
	assert.Equal(t, types.OrderClosed, order.State)
}

func createLease(t testing.TB, suite *state.TestSuite) v1.LeaseID {
	t.Helper()
	ctx := suite.Context()
	bid, order := createBid(t, suite)
	keeper := suite.MarketKeeper()

	err := keeper.CreateLease(ctx, bid)
	require.NoError(t, err)

	keeper.OnBidMatched(ctx, bid)
	keeper.OnOrderMatched(ctx, order)

	owner, err := sdk.AccAddressFromBech32(bid.ID.Owner)
	require.NoError(t, err)

	defaultDeposit, err := dtypes.DefaultParams().MinDepositFor("uakt")
	require.NoError(t, err)

	msg := &dtypes.MsgCreateDeployment{
		ID: order.ID.GroupID().DeploymentID(),
		Deposit: deposit.Deposit{
			Amount:  defaultDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		}}

	deposits, err := aauthz.AuthorizeDeposit(ctx, suite.AuthzKeeper(), suite.BankKeeper(), msg)
	require.NoError(t, err)

	err = suite.EscrowKeeper().AccountCreate(
		ctx,
		dtypes.EscrowAccountForDeployment(bid.ID.DeploymentID()),
		owner,
		deposits,
	)
	require.NoError(t, err)

	provider, err := sdk.AccAddressFromBech32(bid.ID.Provider)
	require.NoError(t, err)

	err = suite.EscrowKeeper().PaymentCreate(
		ctx,
		dtypes.EscrowAccountForDeployment(bid.ID.DeploymentID()),
		types.EscrowPaymentForLease(bid.ID.LeaseID()),
		provider,
		bid.Price,
	)
	require.NoError(t, err)

	return bid.ID.LeaseID()
}

func createBid(t testing.TB, suite *state.TestSuite) (types.Bid, types.Order) {
	t.Helper()
	ctx := suite.Context()
	order, gspec := createOrder(t, suite.Context(), suite.MarketKeeper())
	provider := testutil.AccAddress(t)
	price := testutil.AkashDecCoinRandom(t)
	roffer := types.ResourceOfferFromRU(gspec.Resources)

	bidID := v1.MakeBidID(order.ID, provider)

	bid, err := suite.MarketKeeper().CreateBid(ctx, bidID, price, roffer)
	require.NoError(t, err)
	assert.Equal(t, order.ID, bid.ID.OrderID())
	assert.Equal(t, price, bid.Price)
	assert.Equal(t, provider.String(), bid.ID.Provider)

	msg := &types.MsgCreateBid{
		ID: bidID,
		Deposit: deposit.Deposit{
			Amount:  types.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		}}

	deposits, err := aauthz.AuthorizeDeposit(ctx, suite.AuthzKeeper(), suite.BankKeeper(), msg)
	require.NoError(t, err)

	err = suite.EscrowKeeper().AccountCreate(
		ctx,
		types.EscrowAccountForBid(bid.ID),
		provider,
		deposits,
	)
	require.NoError(t, err)

	return bid, order
}

func createOrder(t testing.TB, ctx sdk.Context, keeper keeper.IKeeper) (types.Order, dtypes.GroupSpec) {
	t.Helper()
	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)

	order, err := keeper.CreateOrder(ctx, group.ID, group.GroupSpec)
	require.NoError(t, err)

	require.Equal(t, group.ID, order.ID.GroupID())
	require.Equal(t, uint32(1), order.ID.OSeq)
	require.Equal(t, types.OrderOpen, order.State)
	return order, group.GroupSpec
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.IKeeper, *state.TestSuite) {
	t.Helper()

	suite := state.SetupTestSuite(t)
	return suite.Context(), suite.MarketKeeper(), suite
}

package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mvbeta "pkg.akt.dev/go/node/market/v1beta5"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"
	"pkg.akt.dev/go/testutil"

	"pkg.akt.dev/node/v2/testutil/state"
	"pkg.akt.dev/node/v2/x/market/keeper"
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
	keeper.WithOrders(ctx, func(result mvbeta.Order) bool {
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
	keeper.WithOrdersForGroup(ctx, order.ID.GroupID(), mvbeta.OrderOpen, func(result mvbeta.Order) bool {
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
	keeper.WithBids(ctx, func(result mvbeta.Bid) bool {
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

	keeper.WithBidsForOrder(ctx, bid.ID.OrderID(), mvbeta.BidOpen, func(result mvbeta.Bid) bool {
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
	keeper.WithLeases(ctx, func(result mv1.Lease) bool {
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

	result, ok := keeper.LeaseForOrder(ctx, mvbeta.BidActive, id.OrderID())
	assert.True(t, ok)

	assert.Equal(t, id, result.ID)

	// no match
	{
		bid, _ := createBid(t, suite)
		_, ok := keeper.LeaseForOrder(ctx, mvbeta.BidActive, bid.ID.OrderID())
		assert.False(t, ok)
	}
}

func Test_OnOrderMatched(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	result, ok := keeper.GetOrder(ctx, id.OrderID())
	require.True(t, ok)
	assert.Equal(t, mvbeta.OrderActive, result.State)
}

func Test_OnBidMatched(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	id := createLease(t, suite)

	result, ok := keeper.GetBid(ctx, id.BidID())
	require.True(t, ok)
	assert.Equal(t, mvbeta.BidActive, result.State)
}

func Test_OnBidLost(t *testing.T) {
	ctx, keeper, suite := setupKeeper(t)
	bid, _ := createBid(t, suite)

	keeper.OnBidLost(ctx, bid)
	result, ok := keeper.GetBid(ctx, bid.ID)
	require.True(t, ok)
	assert.Equal(t, mvbeta.BidLost, result.State)
}

func Test_OnOrderClosed(t *testing.T) {
	ctx, keeper, _ := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	err := keeper.OnOrderClosed(ctx, order)
	require.NoError(t, err)

	result, ok := keeper.GetOrder(ctx, order.ID)
	require.True(t, ok)
	assert.Equal(t, mvbeta.OrderClosed, result.State)
}

func Test_OnLeaseClosed(t *testing.T) {
	tests := []struct {
		name          string
		state         mv1.Lease_State
		reason        mv1.LeaseClosedReason
		expectedState mv1.Lease_State
	}{
		{
			name:          "closed_with_unspecified_reason",
			state:         mv1.LeaseClosed,
			reason:        mv1.LeaseClosedReasonUnspecified,
			expectedState: mv1.LeaseClosed,
		},
		{
			name:          "closed_by_owner",
			state:         mv1.LeaseClosed,
			reason:        mv1.LeaseClosedReasonOwner,
			expectedState: mv1.LeaseClosed,
		},
		{
			name:          "insufficient_funds",
			state:         mv1.LeaseInsufficientFunds,
			reason:        mv1.LeaseClosedReasonInsufficientFunds,
			expectedState: mv1.LeaseInsufficientFunds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, keeper, suite := setupKeeper(t)
			suite.SetBlockHeight(1)
			id := createLease(t, suite)

			lease, ok := keeper.GetLease(suite.Context(), id)
			require.True(t, ok)
			require.Equal(t, int64(0), lease.ClosedOn)

			const testBlockHeight = 1337
			suite.SetBlockHeight(testBlockHeight)

			require.Equal(t, mv1.LeaseActive, lease.State)
			err := keeper.OnLeaseClosed(suite.Context(), lease, tt.state, tt.reason)
			require.NoError(t, err)

			result, ok := keeper.GetLease(suite.Context(), id)
			require.True(t, ok)
			assert.Equal(t, tt.expectedState, result.State)
			assert.Equal(t, int64(testBlockHeight), result.ClosedOn)
			assert.Equal(t, tt.reason, result.Reason)
		})
	}
}

func Test_OnLeaseClosed_Idempotency(t *testing.T) {
	tests := []struct {
		name          string
		firstState    mv1.Lease_State
		firstReason   mv1.LeaseClosedReason
		secondState   mv1.Lease_State
		secondReason  mv1.LeaseClosedReason
		expectedState mv1.Lease_State
	}{
		{
			name:          "same_state_same_reason",
			firstState:    mv1.LeaseClosed,
			firstReason:   mv1.LeaseClosedReasonOwner,
			secondState:   mv1.LeaseClosed,
			secondReason:  mv1.LeaseClosedReasonOwner,
			expectedState: mv1.LeaseClosed,
		},
		{
			name:          "same_state_different_reason",
			firstState:    mv1.LeaseClosed,
			firstReason:   mv1.LeaseClosedReasonOwner,
			secondState:   mv1.LeaseClosed,
			secondReason:  mv1.LeaseClosedReasonUnspecified,
			expectedState: mv1.LeaseClosed,
		},
		{
			name:          "closed_to_insufficient_funds_blocked",
			firstState:    mv1.LeaseClosed,
			firstReason:   mv1.LeaseClosedReasonOwner,
			secondState:   mv1.LeaseInsufficientFunds,
			secondReason:  mv1.LeaseClosedReasonInsufficientFunds,
			expectedState: mv1.LeaseClosed,
		},
		{
			name:          "insufficient_funds_to_closed_blocked",
			firstState:    mv1.LeaseInsufficientFunds,
			firstReason:   mv1.LeaseClosedReasonInsufficientFunds,
			secondState:   mv1.LeaseClosed,
			secondReason:  mv1.LeaseClosedReasonOwner,
			expectedState: mv1.LeaseInsufficientFunds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, keeper, suite := setupKeeper(t)
			suite.SetBlockHeight(1)
			id := createLease(t, suite)

			lease, ok := keeper.GetLease(suite.Context(), id)
			require.True(t, ok)

			const firstBlockHeight = 100
			suite.SetBlockHeight(firstBlockHeight)
			err := keeper.OnLeaseClosed(suite.Context(), lease, tt.firstState, tt.firstReason)
			require.NoError(t, err)

			result, ok := keeper.GetLease(suite.Context(), id)
			require.True(t, ok)
			assert.Equal(t, tt.firstState, result.State)
			assert.Equal(t, int64(firstBlockHeight), result.ClosedOn)
			assert.Equal(t, tt.firstReason, result.Reason)

			const secondBlockHeight = 200
			suite.SetBlockHeight(secondBlockHeight)
			err = keeper.OnLeaseClosed(suite.Context(), result, tt.secondState, tt.secondReason)
			require.NoError(t, err)

			result2, ok := keeper.GetLease(suite.Context(), id)
			require.True(t, ok)
			assert.Equal(t, tt.expectedState, result2.State)
			assert.Equal(t, int64(firstBlockHeight), result2.ClosedOn)
			assert.Equal(t, tt.firstReason, result2.Reason)
		})
	}
}

func Test_OnGroupClosed(t *testing.T) {
	tests := []struct {
		name               string
		groupState         dtypes.Group_State
		expectedLeaseState mv1.Lease_State
		expectedReason     mv1.LeaseClosedReason
	}{
		{
			name:               "group_closed",
			groupState:         dtypes.GroupClosed,
			expectedLeaseState: mv1.LeaseClosed,
			expectedReason:     mv1.LeaseClosedReasonOwner,
		},
		{
			name:               "group_insufficient_funds",
			groupState:         dtypes.GroupInsufficientFunds,
			expectedLeaseState: mv1.LeaseInsufficientFunds,
			expectedReason:     mv1.LeaseClosedReasonInsufficientFunds,
		},
		{
			name:               "group_paused",
			groupState:         dtypes.GroupPaused,
			expectedLeaseState: mv1.LeaseClosed,
			expectedReason:     mv1.LeaseClosedReasonOwner,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, keeper, suite := setupKeeper(t)
			id := createLease(t, suite)

			gid := id.BidID().GroupID()
			deployment := testutil.Deployment(t)
			deployment.ID = gid.DeploymentID()
			group := testutil.DeploymentGroup(t, deployment.ID, gid.GSeq)
			err := suite.DeploymentKeeper().Create(suite.Context(), deployment, []dtypes.Group{group})
			require.NoError(t, err)
			const testBlockHeight = 133
			suite.SetBlockHeight(testBlockHeight)
			err = keeper.OnGroupClosed(suite.Context(), gid, tt.groupState)
			require.NoError(t, err)

			lease, ok := keeper.GetLease(suite.Context(), id)
			require.True(t, ok)
			assert.Equal(t, tt.expectedLeaseState, lease.State)
			assert.Equal(t, tt.expectedReason, lease.Reason)
			assert.Equal(t, int64(testBlockHeight), lease.ClosedOn)

			bid, ok := keeper.GetBid(suite.Context(), id.BidID())
			require.True(t, ok)
			assert.Equal(t, mvbeta.BidClosed, bid.State)

			order, ok := keeper.GetOrder(suite.Context(), id.OrderID())
			require.True(t, ok)
			assert.Equal(t, mvbeta.OrderClosed, order.State)
		})
	}
}

func createLease(t testing.TB, suite *state.TestSuite) mv1.LeaseID {
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

	deposits, err := suite.EscrowKeeper().AuthorizeDeposits(ctx, msg)
	require.NoError(t, err)

	err = suite.EscrowKeeper().AccountCreate(
		ctx,
		bid.ID.DeploymentID().ToEscrowAccountID(),
		owner,
		deposits,
	)
	require.NoError(t, err)

	provider, err := sdk.AccAddressFromBech32(bid.ID.Provider)
	require.NoError(t, err)

	err = suite.EscrowKeeper().PaymentCreate(
		ctx,
		bid.ID.LeaseID().ToEscrowPaymentID(),
		provider,
		bid.Price,
	)
	require.NoError(t, err)

	return bid.ID.LeaseID()
}

func createBid(t testing.TB, suite *state.TestSuite) (mvbeta.Bid, mvbeta.Order) {
	t.Helper()
	ctx := suite.Context()
	order, gspec := createOrder(t, suite.Context(), suite.MarketKeeper())
	provider := testutil.AccAddress(t)
	price := testutil.AkashDecCoinRandom(t)
	roffer := mvbeta.ResourceOfferFromRU(gspec.Resources)

	bidID := mv1.MakeBidID(order.ID, provider)

	bid, err := suite.MarketKeeper().CreateBid(ctx, bidID, price, roffer)
	require.NoError(t, err)
	assert.Equal(t, order.ID, bid.ID.OrderID())
	assert.Equal(t, price, bid.Price)
	assert.Equal(t, provider.String(), bid.ID.Provider)

	msg := &mvbeta.MsgCreateBid{
		ID: bidID,
		Deposit: deposit.Deposit{
			Amount:  mvbeta.DefaultBidMinDeposit,
			Sources: deposit.Sources{deposit.SourceBalance},
		}}

	deposits, err := suite.EscrowKeeper().AuthorizeDeposits(ctx, msg)
	require.NoError(t, err)

	err = suite.EscrowKeeper().AccountCreate(
		ctx,
		bid.ID.ToEscrowAccountID(),
		provider,
		deposits,
	)
	require.NoError(t, err)

	return bid, order
}

func createOrder(t testing.TB, ctx sdk.Context, keeper keeper.IKeeper) (mvbeta.Order, dtypes.GroupSpec) {
	t.Helper()
	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)

	order, err := keeper.CreateOrder(ctx, group.ID, group.GroupSpec)
	require.NoError(t, err)

	require.Equal(t, group.ID, order.ID.GroupID())
	require.Equal(t, uint32(1), order.ID.OSeq)
	require.Equal(t, mvbeta.OrderOpen, order.State)
	return order, group.GroupSpec
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.IKeeper, *state.TestSuite) {
	t.Helper()

	suite := state.SetupTestSuite(t)
	suite.PrepareMocks(func(ts *state.TestSuite) {
		bkeeper := ts.BankKeeper()

		bkeeper.
			On("SendCoinsFromAccountToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToAccount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.
			On("SendCoinsFromModuleToModule", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil)

		bkeeper.On("BurnCoins", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
		bkeeper.On("MintCoins", mock.Anything, mock.Anything, mock.Anything).
			Return(nil)
	})

	return suite.Context(), suite.MarketKeeper(), suite
}

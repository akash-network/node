package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/rand"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/testutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
)

func Test_CreateOrder(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	order, gspec := createOrder(t, ctx, keeper)

	// assert one active for group
	_, err := keeper.CreateOrder(ctx, order.ID().GroupID(), gspec)
	require.Error(t, err)
}

func Test_GetOrder(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)
	result, ok := keeper.GetOrder(ctx, order.ID())
	require.True(t, ok)
	require.Equal(t, order, result)

	// assert non-existent order fails
	{
		_, ok := keeper.GetOrder(ctx, testutil.OrderID(t))
		require.False(t, ok)
	}
}

func Test_WithOrders(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	count := 0
	keeper.WithOrders(ctx, func(result types.Order) bool {
		if assert.Equal(t, order.ID(), result.ID()) {
			count++
		}
		return false
	})

	assert.Equal(t, 1, count)

	t.Run("open orders", func(t *testing.T) {
		openCount := 0
		keeper.WithOpenOrders(ctx, func(result types.Order) bool {
			if assert.Equal(t, order.ID(), result.ID()) {
				openCount++
			}
			return false
		})
		assert.Equal(t, openCount, 1)
	})
}

func Test_WithOrdersForGroup(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	// create extra orders
	createOrder(t, ctx, keeper)

	count := 0
	keeper.WithOrdersForGroup(ctx, order.ID().GroupID(), func(result types.Order) bool {
		if assert.Equal(t, order.ID(), result.ID()) {
			count++
		}
		return false
	})

	assert.Equal(t, 1, count)
}

func Test_CreateBid(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	createBid(t, ctx, keeper)
}

func Test_GetBid(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	bid, _ := createBid(t, ctx, keeper)

	result, ok := keeper.GetBid(ctx, bid.ID())
	require.True(t, ok)
	assert.Equal(t, bid, result)

	// non-existent
	{
		_, ok := keeper.GetBid(ctx, testutil.BidID(t))
		require.False(t, ok)
	}
}

func Test_WithBids(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	bid, _ := createBid(t, ctx, keeper)
	count := 0
	keeper.WithBids(ctx, func(result types.Bid) bool {
		if assert.Equal(t, bid.ID(), result.ID()) {
			count++
		}
		return false
	})
	assert.Equal(t, 1, count)
}

func Test_WithBidsForOrder(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	bid, _ := createBid(t, ctx, keeper)

	// create extra bids
	createBid(t, ctx, keeper)
	createBid(t, ctx, keeper)

	count := 0
	keeper.WithBidsForOrder(ctx, bid.ID().OrderID(), func(result types.Bid) bool {
		if assert.Equal(t, bid.ID(), result.ID()) {
			count++
		}
		return false
	})
	assert.Equal(t, 1, count)
}

func Test_GetLease(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	lease, ok := keeper.GetLease(ctx, id)
	assert.True(t, ok)
	assert.Equal(t, id, lease.ID())

	// non-existent
	{
		_, ok := keeper.GetLease(ctx, testutil.LeaseID(t))
		require.False(t, ok)
	}
}

func Test_WithLeases(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	count := 0
	keeper.WithLeases(ctx, func(result types.Lease) bool {
		if assert.Equal(t, id, result.ID()) {
			count++
		}
		return false
	})
	assert.Equal(t, 1, count)

	t.Run("active-count", func(t *testing.T) {
		activeCount := 0
		keeper.WithActiveLeases(ctx, func(result types.Lease) bool {
			if assert.Equal(t, id, result.ID()) {
				activeCount++
			}
			return false
		})

		assert.Equal(t, 1, activeCount)
	})
}

func Test_LeaseForOrder(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	// extra leases
	createLease(t, ctx, keeper)
	createLease(t, ctx, keeper)

	result, ok := keeper.LeaseForOrder(ctx, id.OrderID())
	assert.True(t, ok)

	assert.Equal(t, id, result.ID())

	// no match
	{
		bid, _ := createBid(t, ctx, keeper)
		_, ok := keeper.LeaseForOrder(ctx, bid.ID().OrderID())
		assert.False(t, ok)
	}
}

func Test_OnOrderMatched(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	result, ok := keeper.GetOrder(ctx, id.OrderID())
	require.True(t, ok)
	assert.Equal(t, types.OrderMatched, result.State)
}

func Test_OnBidMatched(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	result, ok := keeper.GetBid(ctx, id.BidID())
	require.True(t, ok)
	assert.Equal(t, types.BidMatched, result.State)
}

func Test_OnBidLost(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	bid, _ := createBid(t, ctx, keeper)

	keeper.OnBidLost(ctx, bid)
	result, ok := keeper.GetBid(ctx, bid.ID())
	require.True(t, ok)
	assert.Equal(t, types.BidLost, result.State)
}

func Test_OnOrderClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	order, _ := createOrder(t, ctx, keeper)

	keeper.OnOrderClosed(ctx, order)

	result, ok := keeper.GetOrder(ctx, order.ID())
	require.True(t, ok)
	assert.Equal(t, types.OrderClosed, result.State)
}

func Test_OnInsufficientFunds(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	lease, ok := keeper.GetLease(ctx, id)
	require.True(t, ok)

	keeper.OnInsufficientFunds(ctx, lease)

	result, ok := keeper.GetLease(ctx, id)
	require.True(t, ok)
	assert.Equal(t, types.LeaseInsufficientFunds, result.State)
}

func Test_OnLeaseClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	lease, ok := keeper.GetLease(ctx, id)
	require.True(t, ok)

	keeper.OnLeaseClosed(ctx, lease)

	result, ok := keeper.GetLease(ctx, id)
	require.True(t, ok)
	assert.Equal(t, types.LeaseClosed, result.State)
}

func Test_OnGroupClosed(t *testing.T) {
	ctx, keeper := setupKeeper(t)
	id := createLease(t, ctx, keeper)

	keeper.OnGroupClosed(ctx, id.BidID().GroupID())

	lease, ok := keeper.GetLease(ctx, id)
	require.True(t, ok)
	assert.Equal(t, types.LeaseClosed, lease.State)

	bid, ok := keeper.GetBid(ctx, id.BidID())
	require.True(t, ok)
	assert.Equal(t, types.BidClosed, bid.State)

	order, ok := keeper.GetOrder(ctx, id.OrderID())
	require.True(t, ok)
	assert.Equal(t, types.OrderClosed, order.State)
}

func createLease(t testing.TB, ctx sdk.Context, keeper keeper.Keeper) types.LeaseID {
	t.Helper()
	bid, order := createBid(t, ctx, keeper)
	keeper.CreateLease(ctx, bid)
	keeper.OnBidMatched(ctx, bid)
	keeper.OnOrderMatched(ctx, order)
	lid := types.MakeLeaseID(bid.ID())
	return lid
}

func createBid(t testing.TB, ctx sdk.Context, keeper keeper.Keeper) (types.Bid, types.Order) {
	t.Helper()
	order, _ := createOrder(t, ctx, keeper)
	provider := testutil.AccAddress(t)
	price := sdk.NewCoin("foo", sdk.NewInt(int64(rand.Uint16())))
	bid, err := keeper.CreateBid(ctx, order.ID(), provider, price)
	require.NoError(t, err)
	assert.Equal(t, order.ID(), bid.ID().OrderID())
	assert.Equal(t, price, bid.Price)
	assert.Equal(t, provider.String(), bid.ID().Provider)
	return bid, order
}

func createOrder(t testing.TB, ctx sdk.Context, keeper keeper.Keeper) (types.Order, dtypes.GroupSpec) {
	t.Helper()
	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)

	order, err := keeper.CreateOrder(ctx, group.ID(), group.GroupSpec)
	require.NoError(t, err)

	require.Equal(t, group.ID(), order.ID().GroupID())
	require.Equal(t, uint32(1), order.ID().OSeq)
	require.Equal(t, types.OrderOpen, order.State)
	return order, group.GroupSpec
}

func setupKeeper(t testing.TB) (sdk.Context, keeper.Keeper) {
	t.Helper()
	key := sdk.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	ms := store.NewCommitMultiStore(db)
	ms.MountStoreWithDB(key, sdk.StoreTypeIAVL, db)
	require.NoError(t, ms.LoadLatestVersion())
	ctx := sdk.NewContext(ms, tmproto.Header{Time: time.Unix(0, 0)}, false, testutil.Logger(t))
	return ctx, keeper.NewKeeper(types.ModuleCdc, key)
}

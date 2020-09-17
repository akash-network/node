package handler_test

import (
	"math"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/store"
	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/tendermint/tendermint/libs/rand"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/types"
	dkeeper "github.com/ovrclk/akash/x/deployment/keeper"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/handler"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
	pkeeper "github.com/ovrclk/akash/x/provider/keeper"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

type testSuite struct {
	t       *testing.T
	ms      sdk.CommitMultiStore
	ctx     sdk.Context
	mkeeper keeper.Keeper
	dkeeper dkeeper.Keeper
	pkeeper pkeeper.Keeper
	bkeeper bankkeeper.Keeper

	handler sdk.Handler
}

func setupTestSuite(t *testing.T) *testSuite {
	suite := &testSuite{
		t: t,
	}

	mKey := sdk.NewKVStoreKey(types.StoreKey)
	dKey := sdk.NewKVStoreKey(dtypes.StoreKey)
	pKey := sdk.NewKVStoreKey(ptypes.StoreKey)

	db := dbm.NewMemDB()
	suite.ms = store.NewCommitMultiStore(db)
	suite.ms.MountStoreWithDB(mKey, sdk.StoreTypeIAVL, db)
	suite.ms.MountStoreWithDB(dKey, sdk.StoreTypeIAVL, db)
	suite.ms.MountStoreWithDB(pKey, sdk.StoreTypeIAVL, db)

	err := suite.ms.LoadLatestVersion()
	require.NoError(t, err)

	suite.ctx = sdk.NewContext(suite.ms, tmproto.Header{}, true, testutil.Logger(t))

	suite.mkeeper = keeper.NewKeeper(types.ModuleCdc, mKey)
	suite.dkeeper = dkeeper.NewKeeper(types.ModuleCdc, dKey)
	suite.pkeeper = pkeeper.NewKeeper(types.ModuleCdc, pKey)

	suite.handler = handler.NewHandler(handler.Keepers{
		Market:     suite.mkeeper,
		Deployment: suite.dkeeper,
		Provider:   suite.pkeeper,
		Bank:       suite.bkeeper,
	})

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.handler(suite.ctx, sdk.Msg(sdktestdata.NewTestMsg()))
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCreateBidValid(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	provider := suite.createProvider(gspec.Requirements).Owner

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: provider,
		Price:    sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(1)),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	bid := types.MakeBidID(order.ID(), provider)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseMarketEvent(t, res.Events[2:])
		require.IsType(t, types.EventBidCreated{}, iev)

		dev := iev.(types.EventBidCreated)

		require.Equal(t, bid, dev.ID)
	})

	_, found := suite.mkeeper.GetBid(suite.ctx, bid)
	require.True(t, found)
}

func TestCreateBidInvalidPrice(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(nil)

	provider := suite.createProvider(gspec.Requirements).Owner

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: provider,
		Price:    sdk.Coin{},
	}
	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.Error(t, err)
	require.EqualError(t, err, types.ErrBidInvalidPrice.Error())

	_, found := suite.mkeeper.GetBid(suite.ctx, types.MakeBidID(order.ID(), provider))
	require.False(t, found)
}

func TestCreateBidNonExistingOrder(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &types.MsgCreateBid{
		Order:    types.OrderID{},
		Provider: nil,
		Price:    sdk.Coin{},
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrInvalidOrder.Error())

	_, found := suite.mkeeper.GetBid(suite.ctx, types.MakeBidID(msg.Order, msg.Provider))
	require.False(t, found)
}

func TestCreateBidClosedOrder(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(nil)

	suite.mkeeper.OnOrderClosed(suite.ctx, order)

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(gspec.Requirements).Owner,
		Price:    sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(math.MaxInt64)),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrOrderClosed.Error())
}

func TestCreateBidOverprice(t *testing.T) {
	suite := setupTestSuite(t)

	resources := []dtypes.Resource{
		{
			Price: sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(1)),
		},
	}
	order, gspec := suite.createOrder(resources)

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(gspec.Requirements).Owner,
		Price:    sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(math.MaxInt64)),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrBidOverOrder.Error())
}

func TestCreateBidInvalidProvider(t *testing.T) {
	suite := setupTestSuite(t)

	order, _ := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: nil,
		Price:    sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(1)),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrEmptyProvider.Error())
}

func TestCreateBidInvalidAttributes(t *testing.T) {
	suite := setupTestSuite(t)

	order, _ := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(testutil.Attributes(t)).Owner,
		Price:    sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(1)),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrAttributeMismatch.Error())
}

func TestCreateBidAlreadyExists(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(gspec.Requirements).Owner,
		Price:    sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(1)),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	res, err = suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrBidExists.Error())
}

func TestCloseOrderNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	dgroup := testutil.DeploymentGroup(suite.t, testutil.DeploymentID(suite.t), 0)
	msg := &types.MsgCloseOrder{
		OrderID: types.MakeOrderID(dgroup.ID(), 1),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrUnknownOrder.Error())
}

func TestCloseOrderWithoutLease(t *testing.T) {
	suite := setupTestSuite(t)

	order, _ := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCloseOrder{
		OrderID: order.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrNoLeaseForOrder.Error())
}

func TestCloseOrderValid(t *testing.T) {
	suite := setupTestSuite(t)

	_, _, order := suite.createLease()

	msg := &types.MsgCloseOrder{
		OrderID: order.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseMarketEvent(t, res.Events[3:4])
		require.IsType(t, types.EventOrderClosed{}, iev)

		dev := iev.(types.EventOrderClosed)

		require.Equal(t, msg.OrderID, dev.ID)
	})
}

func TestCloseBidNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	provider := suite.createProvider(gspec.Requirements).Owner

	msg := &types.MsgCloseBid{
		BidID: types.MakeBidID(order.ID(), provider),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrUnknownBid.Error())
}

func TestCloseBidUnknownLease(t *testing.T) {
	suite := setupTestSuite(t)

	bid, _ := suite.createBid()

	suite.mkeeper.OnBidMatched(suite.ctx, bid)

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrUnknownLeaseForBid.Error())
}

func TestCloseBidValid(t *testing.T) {
	suite := setupTestSuite(t)

	_, bid, _ := suite.createLease()

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseMarketEvent(t, res.Events[3:4])
		require.IsType(t, types.EventBidClosed{}, iev)

		dev := iev.(types.EventBidClosed)

		require.Equal(t, msg.BidID, dev.ID)
	})
}

func TestCloseBidWithStateOpen(t *testing.T) {
	suite := setupTestSuite(t)

	bid, _ := suite.createBid()

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		iev := testutil.ParseMarketEvent(t, res.Events[2:])
		require.IsType(t, types.EventBidClosed{}, iev)

		dev := iev.(types.EventBidClosed)

		require.Equal(t, msg.BidID, dev.ID)
	})
}

func TestCloseBidNotActiveLease(t *testing.T) {
	suite := setupTestSuite(t)

	lease, bid, _ := suite.createLease()

	suite.mkeeper.OnLeaseClosed(suite.ctx, types.Lease{
		LeaseID: lease,
	})
	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrLeaseNotActive.Error())
}

func TestCloseBidUnknownOrder(t *testing.T) {
	suite := setupTestSuite(t)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)
	orderID := types.MakeOrderID(group.ID(), 1)
	provider := testutil.AccAddress(t)
	price := sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(int64(rand.Uint16())))

	bid, err := suite.mkeeper.CreateBid(suite.ctx, orderID, provider, price)
	require.NoError(t, err)

	suite.mkeeper.CreateLease(suite.ctx, bid)

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.ctx, msg)
	require.Nil(t, res)
	require.EqualError(t, err, types.ErrUnknownOrderForBid.Error())
}

func (st *testSuite) createLease() (types.LeaseID, types.Bid, types.Order) {
	st.t.Helper()
	bid, order := st.createBid()

	st.mkeeper.CreateLease(st.ctx, bid)
	st.mkeeper.OnBidMatched(st.ctx, bid)
	st.mkeeper.OnOrderMatched(st.ctx, order)

	lid := types.MakeLeaseID(bid.ID())
	return lid, bid, order
}

func (st *testSuite) createBid() (types.Bid, types.Order) {
	st.t.Helper()
	order, _ := st.createOrder(testutil.Resources(st.t))
	provider := testutil.AccAddress(st.t)
	price := sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(int64(rand.Uint16())))
	bid, err := st.mkeeper.CreateBid(st.ctx, order.ID(), provider, price)
	require.NoError(st.t, err)
	require.Equal(st.t, order.ID(), bid.ID().OrderID())
	require.Equal(st.t, price, bid.Price)
	require.Equal(st.t, provider, bid.ID().Provider)
	return bid, order
}

func (st *testSuite) createOrder(resources []dtypes.Resource) (types.Order, dtypes.GroupSpec) {
	st.t.Helper()
	group := testutil.DeploymentGroup(st.t, testutil.DeploymentID(st.t), 0)

	group.GroupSpec.Resources = resources
	order, err := st.mkeeper.CreateOrder(st.ctx, group.ID(), group.GroupSpec)
	require.NoError(st.t, err)
	require.Equal(st.t, group.ID(), order.ID().GroupID())
	require.Equal(st.t, uint32(1), order.ID().OSeq)
	require.Equal(st.t, types.OrderOpen, order.State)

	return order, group.GroupSpec
}

func (st *testSuite) createProvider(attr []atypes.Attribute) ptypes.Provider {
	st.t.Helper()

	prov := ptypes.Provider{
		Owner:      testutil.AccAddress(st.t),
		HostURI:    "thinker://tailor.soldier?sailor",
		Attributes: attr,
	}

	err := st.pkeeper.Create(st.ctx, prov)
	require.NoError(st.t, err)

	return prov
}

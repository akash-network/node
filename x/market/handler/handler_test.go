package handler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sdktestdata "github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/tendermint/tendermint/libs/rand"

	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	types "github.com/akash-network/akash-api/go/node/market/v1beta3"

	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	akashtypes "github.com/akash-network/akash-api/go/node/types/v1beta3"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	"github.com/akash-network/node/x/market/handler"
)

type testSuite struct {
	*state.TestSuite
	handler sdk.Handler
	t       testing.TB
}

func setupTestSuite(t *testing.T) *testSuite {
	ssuite := state.SetupTestSuite(t)
	suite := &testSuite{
		t:         t,
		TestSuite: ssuite,
		handler: handler.NewHandler(handler.Keepers{
			Escrow:     ssuite.EscrowKeeper(),
			Audit:      ssuite.AuditKeeper(),
			Market:     ssuite.MarketKeeper(),
			Deployment: ssuite.DeploymentKeeper(),
			Provider:   ssuite.ProviderKeeper(),
			// unused?
			// Bank:       ssuite.BankKeeper(),
		}),
	}

	return suite
}

func TestProviderBadMessageType(t *testing.T) {
	suite := setupTestSuite(t)

	res, err := suite.handler(suite.Context(), sdk.Msg(sdktestdata.NewTestMsg()))
	require.Nil(t, res)
	require.Error(t, err)
	require.True(t, errors.Is(err, sdkerrors.ErrUnknownRequest))
}

func TestCreateBidValid(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	provider := suite.createProvider(gspec.Requirements.Attributes).Owner

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: provider,
		Price:    sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(1)),
		Deposit:  types.DefaultBidMinDeposit,
	}

	res, err := suite.handler(suite.Context(), msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	bid := types.MakeBidID(order.ID(), providerAddr)

	t.Run("ensure event created", func(t *testing.T) {
		t.Skip("EVENTS TESTING")
		iev := testutil.ParseMarketEvent(t, res.Events[2:])
		require.IsType(t, types.EventBidCreated{}, iev)

		dev := iev.(types.EventBidCreated)

		require.Equal(t, bid, dev.ID)
	})

	_, found := suite.MarketKeeper().GetBid(suite.Context(), bid)
	require.True(t, found)
}

func TestCreateBidInvalidPrice(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(nil)

	provider := suite.createProvider(gspec.Requirements.Attributes).Owner

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: provider,
		Price:    sdk.DecCoin{},
	}
	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)

	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	_, found := suite.MarketKeeper().GetBid(suite.Context(), types.MakeBidID(order.ID(), providerAddr))
	require.False(t, found)
}

func TestCreateBidNonExistingOrder(t *testing.T) {
	suite := setupTestSuite(t)

	msg := &types.MsgCreateBid{
		Order:    types.OrderID{Owner: testutil.AccAddress(t).String()},
		Provider: testutil.AccAddress(t).String(),
		Price:    testutil.AkashDecCoinRandom(t),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)

	providerAddr, _ := sdk.AccAddressFromBech32(msg.Provider)

	_, found := suite.MarketKeeper().GetBid(suite.Context(), types.MakeBidID(msg.Order, providerAddr))
	require.False(t, found)
}

func TestCreateBidClosedOrder(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(nil)

	suite.MarketKeeper().OnOrderClosed(suite.Context(), order)

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(gspec.Requirements.Attributes).Owner,
		Price:    sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(math.MaxInt64)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidOverprice(t *testing.T) {
	suite := setupTestSuite(t)

	resources := []dtypes.Resource{
		{
			Price: sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(1)),
		},
	}
	order, gspec := suite.createOrder(resources)

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(gspec.Requirements.Attributes).Owner,
		Price:    sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(math.MaxInt64)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidInvalidProvider(t *testing.T) {
	suite := setupTestSuite(t)

	order, _ := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: "",
		Price:    sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(1)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidInvalidAttributes(t *testing.T) {
	suite := setupTestSuite(t)

	order, _ := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(testutil.Attributes(t)).Owner,
		Price:    sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(1)),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCreateBidAlreadyExists(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	msg := &types.MsgCreateBid{
		Order:    order.ID(),
		Provider: suite.createProvider(gspec.Requirements.Attributes).Owner,
		Price:    sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(1)),
		Deposit:  types.DefaultBidMinDeposit,
	}

	res, err := suite.handler(suite.Context(), msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	res, err = suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCloseOrderNonExisting(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// dgroup := testutil.DeploymentGroup(suite.t, testutil.DeploymentID(suite.t), 0)
	// msg := &types.MsgCloseOrder{
	// 	OrderID: types.MakeOrderID(dgroup.ID(), 1),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.Nil(t, res)
	// require.Error(t, err)
}

func TestCloseOrderWithoutLease(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// order, _ := suite.createOrder(testutil.Resources(t))

	// msg := &types.MsgCloseOrder{
	// 	OrderID: order.ID(),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.Nil(t, res)
	// require.Error(t, err)
}

func TestCloseOrderValid(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// _, _, order := suite.createLease()

	// msg := &types.MsgCloseOrder{
	// 	OrderID: order.ID(),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.NotNil(t, res)
	// require.NoError(t, err)

	// t.Run("ensure event created", func(t *testing.T) {
	// 	iev := testutil.ParseMarketEvent(t, res.Events[3:4])
	// 	require.IsType(t, types.EventOrderClosed{}, iev)

	// 	dev := iev.(types.EventOrderClosed)

	// 	require.Equal(t, msg.OrderID, dev.ID)
	// })
}

func TestCloseBidNonExisting(t *testing.T) {
	suite := setupTestSuite(t)

	order, gspec := suite.createOrder(testutil.Resources(t))

	provider := suite.createProvider(gspec.Requirements.Attributes).Owner

	providerAddr, err := sdk.AccAddressFromBech32(provider)
	require.NoError(t, err)

	msg := &types.MsgCloseBid{
		BidID: types.MakeBidID(order.ID(), providerAddr),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCloseBidUnknownLease(t *testing.T) {
	suite := setupTestSuite(t)

	bid, _ := suite.createBid()

	suite.MarketKeeper().OnBidMatched(suite.Context(), bid)

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func TestCloseBidValid(t *testing.T) {
	suite := setupTestSuite(t)

	_, bid, _ := suite.createLease()

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.Context(), msg)
	assert.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		t.Skip("EVENTS TESTING")
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

	res, err := suite.handler(suite.Context(), msg)
	require.NotNil(t, res)
	require.NoError(t, err)

	t.Run("ensure event created", func(t *testing.T) {
		t.Skip("EVENTS TESTING")
		iev := testutil.ParseMarketEvent(t, res.Events[2:])
		require.IsType(t, types.EventBidClosed{}, iev)

		dev := iev.(types.EventBidClosed)

		require.Equal(t, msg.BidID, dev.ID)
	})
}

func TestCloseBidNotActiveLease(t *testing.T) {
	t.Skip("TODO CLOSE LEASE")
	// suite := setupTestSuite(t)

	// lease, bid, _ := suite.createLease()

	// suite.MarketKeeper().OnLeaseClosed(suite.Context(), types.Lease{
	// 	LeaseID: lease,
	// })
	// msg := &types.MsgCloseBid{
	// 	BidID: bid.ID(),
	// }

	// res, err := suite.handler(suite.Context(), msg)
	// require.Nil(t, res)
	// require.Error(t, err)
}

func TestCloseBidUnknownOrder(t *testing.T) {
	suite := setupTestSuite(t)

	group := testutil.DeploymentGroup(t, testutil.DeploymentID(t), 0)
	orderID := types.MakeOrderID(group.ID(), 1)
	provider := testutil.AccAddress(t)
	price := sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(int64(rand.Uint16())))

	bid, err := suite.MarketKeeper().CreateBid(suite.Context(), orderID, provider, price)
	require.NoError(t, err)

	suite.MarketKeeper().CreateLease(suite.Context(), bid)

	msg := &types.MsgCloseBid{
		BidID: bid.ID(),
	}

	res, err := suite.handler(suite.Context(), msg)
	require.Nil(t, res)
	require.Error(t, err)
}

func (st *testSuite) createLease() (types.LeaseID, types.Bid, types.Order) {
	st.t.Helper()
	bid, order := st.createBid()

	st.MarketKeeper().CreateLease(st.Context(), bid)
	st.MarketKeeper().OnBidMatched(st.Context(), bid)
	st.MarketKeeper().OnOrderMatched(st.Context(), order)

	lid := types.MakeLeaseID(bid.ID())
	return lid, bid, order
}

func (st *testSuite) createBid() (types.Bid, types.Order) {
	st.t.Helper()
	order, _ := st.createOrder(testutil.Resources(st.t))
	provider := testutil.AccAddress(st.t)
	price := sdk.NewDecCoin(testutil.CoinDenom, sdk.NewInt(int64(rand.Uint16())))
	bid, err := st.MarketKeeper().CreateBid(st.Context(), order.ID(), provider, price)
	require.NoError(st.t, err)
	require.Equal(st.t, order.ID(), bid.ID().OrderID())
	require.Equal(st.t, price, bid.Price)
	require.Equal(st.t, provider.String(), bid.ID().Provider)
	return bid, order
}

func (st *testSuite) createOrder(resources []dtypes.Resource) (types.Order, dtypes.GroupSpec) {
	st.t.Helper()

	deployment := testutil.Deployment(st.t)
	group := testutil.DeploymentGroup(st.t, deployment.ID(), 0)
	group.GroupSpec.Resources = resources

	err := st.DeploymentKeeper().Create(st.Context(), deployment, []dtypes.Group{group})
	require.NoError(st.t, err)

	order, err := st.MarketKeeper().CreateOrder(st.Context(), group.ID(), group.GroupSpec)
	require.NoError(st.t, err)
	require.Equal(st.t, group.ID(), order.ID().GroupID())
	require.Equal(st.t, uint32(1), order.ID().OSeq)
	require.Equal(st.t, types.OrderOpen, order.State)

	return order, group.GroupSpec
}

func (st *testSuite) createProvider(attr []akashtypes.Attribute) ptypes.Provider {
	st.t.Helper()

	prov := ptypes.Provider{
		Owner:      testutil.AccAddress(st.t).String(),
		HostURI:    "thinker://tailor.soldier?sailor",
		Attributes: attr,
	}

	err := st.ProviderKeeper().Create(st.Context(), prov)
	require.NoError(st.t, err)

	return prov
}

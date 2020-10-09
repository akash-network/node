package keeper_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/x/market/keeper"
	"github.com/ovrclk/akash/x/market/types"
)

type grpcTestSuite struct {
	t      *testing.T
	app    *app.AkashApp
	ctx    sdk.Context
	keeper keeper.Keeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	suite := &grpcTestSuite{
		t: t,
	}

	suite.app = app.Setup(false)
	suite.ctx, suite.keeper = setupKeeper(t)
	querier := keeper.Querier{Keeper: suite.keeper}

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.app.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, querier)
	suite.queryClient = types.NewQueryClient(queryHelper)

	return suite
}

func TestGRPCQueryOrder(t *testing.T) {
	suite := setupTest(t)

	// creating order
	order, _ := createOrder(t, suite.ctx, suite.keeper)

	var (
		req      *types.QueryOrderRequest
		expOrder types.Order
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryOrderRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &types.QueryOrderRequest{ID: types.OrderID{}}
			},
			false,
		},
		{
			"order not found",
			func() {
				req = &types.QueryOrderRequest{ID: types.OrderID{
					Owner: testutil.AccAddress(t).String(),
					DSeq:  32,
					GSeq:  43,
					OSeq:  25,
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryOrderRequest{ID: order.OrderID}
				expOrder = order
			},
			true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Order(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expOrder, res.Order)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func TestGRPCQueryOrders(t *testing.T) {
	suite := setupTest(t)

	// creating orders with different states
	_, _ = createOrder(t, suite.ctx, suite.keeper)
	order2, _ := createOrder(t, suite.ctx, suite.keeper)
	suite.keeper.OnOrderMatched(suite.ctx, order2)

	var req *types.QueryOrdersRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query orders without any filters and pagination",
			func() {
				req = &types.QueryOrdersRequest{}
			},
			2,
		},
		{
			"query orders with filters having non existent data",
			func() {
				req = &types.QueryOrdersRequest{
					Filters: types.OrderFilters{
						OSeq:  37,
						State: "matched",
					}}
			},
			0,
		},
		{
			"query orders with state filter",
			func() {
				req = &types.QueryOrdersRequest{Filters: types.OrderFilters{State: types.OrderMatched.String()}}
			},
			1,
		},
		{
			"query orders with pagination",
			func() {
				req = &types.QueryOrdersRequest{Pagination: &sdkquery.PageRequest{Limit: 1}}
			},
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Orders(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Orders))
		})
	}
}

func TestGRPCQueryBid(t *testing.T) {
	suite := setupTest(t)

	// creating bid
	bid, _ := createBid(t, suite.ctx, suite.keeper)

	var (
		req    *types.QueryBidRequest
		expBid types.Bid
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryBidRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &types.QueryBidRequest{ID: types.BidID{}}
			},
			false,
		},
		{
			"bid not found",
			func() {
				req = &types.QueryBidRequest{ID: types.BidID{
					Owner:    testutil.AccAddress(t).String(),
					DSeq:     32,
					GSeq:     43,
					OSeq:     25,
					Provider: testutil.AccAddress(t).String(),
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryBidRequest{ID: bid.BidID}
				expBid = bid
			},
			true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Bid(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expBid, res.Bid)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func TestGRPCQueryBids(t *testing.T) {
	suite := setupTest(t)

	// creating bids with different states
	_, _ = createBid(t, suite.ctx, suite.keeper)
	bid2, _ := createBid(t, suite.ctx, suite.keeper)
	suite.keeper.OnBidLost(suite.ctx, bid2)

	var req *types.QueryBidsRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query bids without any filters and pagination",
			func() {
				req = &types.QueryBidsRequest{}
			},
			2,
		},
		{
			"query bids with filters having non existent data",
			func() {
				req = &types.QueryBidsRequest{
					Filters: types.BidFilters{
						OSeq:     37,
						State:    "lost",
						Provider: testutil.AccAddress(t).String(),
					}}
			},
			0,
		},
		{
			"query bids with state filter",
			func() {
				req = &types.QueryBidsRequest{Filters: types.BidFilters{State: types.BidLost.String()}}
			},
			1,
		},
		{
			"query bids with pagination",
			func() {
				req = &types.QueryBidsRequest{Pagination: &sdkquery.PageRequest{Limit: 1}}
			},
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Bids(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Bids))
		})
	}
}

func TestGRPCQueryLease(t *testing.T) {
	suite := setupTest(t)

	// creating lease
	leaseID := createLease(t, suite.ctx, suite.keeper)
	lease, ok := suite.keeper.GetLease(suite.ctx, leaseID)
	require.True(t, ok)

	var (
		req      *types.QueryLeaseRequest
		expLease types.Lease
	)

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"empty request",
			func() {
				req = &types.QueryLeaseRequest{}
			},
			false,
		},
		{
			"invalid request",
			func() {
				req = &types.QueryLeaseRequest{ID: types.LeaseID{}}
			},
			false,
		},
		{
			"lease not found",
			func() {
				req = &types.QueryLeaseRequest{ID: types.LeaseID{
					Owner:    testutil.AccAddress(t).String(),
					DSeq:     32,
					GSeq:     43,
					OSeq:     25,
					Provider: testutil.AccAddress(t).String(),
				}}
			},
			false,
		},
		{
			"success",
			func() {
				req = &types.QueryLeaseRequest{ID: lease.LeaseID}
				expLease = lease
			},
			true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Lease(ctx, req)

			if tc.expPass {
				require.NoError(t, err)
				require.NotNil(t, res)
				require.Equal(t, expLease, res.Lease)
			} else {
				require.Error(t, err)
				require.Nil(t, res)
			}

		})
	}
}

func TestGRPCQueryLeases(t *testing.T) {
	suite := setupTest(t)

	// creating leases with different states
	leaseID := createLease(t, suite.ctx, suite.keeper)
	_, ok := suite.keeper.GetLease(suite.ctx, leaseID)
	require.True(t, ok)

	leaseID2 := createLease(t, suite.ctx, suite.keeper)
	lease2, ok := suite.keeper.GetLease(suite.ctx, leaseID2)
	require.True(t, ok)
	suite.keeper.OnLeaseClosed(suite.ctx, lease2)

	var req *types.QueryLeasesRequest

	testCases := []struct {
		msg      string
		malleate func()
		expLen   int
	}{
		{
			"query leases without any filters and pagination",
			func() {
				req = &types.QueryLeasesRequest{}
			},
			2,
		},
		{
			"query leases with filters having non existent data",
			func() {
				req = &types.QueryLeasesRequest{
					Filters: types.LeaseFilters{
						OSeq:     37,
						State:    "closed",
						Provider: testutil.AccAddress(t).String(),
					}}
			},
			0,
		},
		{
			"query leases with state filter",
			func() {
				req = &types.QueryLeasesRequest{Filters: types.LeaseFilters{State: types.LeaseClosed.String()}}
			},
			1,
		},
		{
			"query leases with pagination",
			func() {
				req = &types.QueryLeasesRequest{Pagination: &sdkquery.PageRequest{Limit: 1}}
			},
			1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("Case %s", tc.msg), func(t *testing.T) {
			tc.malleate()
			ctx := sdk.WrapSDKContext(suite.ctx)

			res, err := suite.queryClient.Leases(ctx, req)

			require.NoError(t, err)
			require.NotNil(t, res)
			require.Equal(t, tc.expLen, len(res.Leases))
		})
	}
}

package keeper_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	"github.com/akash-network/node/testutil"
	"github.com/akash-network/node/testutil/state"
	"github.com/akash-network/node/x/market/keeper"
)

type grpcTestSuite struct {
	*state.TestSuite
	t      *testing.T
	ctx    sdk.Context
	keeper keeper.IKeeper

	queryClient types.QueryClient
}

func setupTest(t *testing.T) *grpcTestSuite {
	ssuite := state.SetupTestSuite(t)

	suite := &grpcTestSuite{
		TestSuite: ssuite,
		t:         t,
		ctx:       ssuite.Context(),
		keeper:    ssuite.MarketKeeper(),
	}

	querier := suite.keeper.NewQuerier()

	queryHelper := baseapp.NewQueryServerTestHelper(suite.ctx, suite.App().InterfaceRegistry())
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
						State: types.OrderActive.String(),
					}}
			},
			0,
		},
		{
			"query orders with state filter",
			func() {
				req = &types.QueryOrdersRequest{Filters: types.OrderFilters{State: types.OrderActive.String()}}
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

type orderFilterModifier struct {
	fieldName string
	f         func(orderID types.OrderID, filter types.OrderFilters) types.OrderFilters
	getField  func(orderID types.OrderID) interface{}
}

type bidFilterModifier struct {
	fieldName string
	f         func(bidID types.BidID, filter types.BidFilters) types.BidFilters
	getField  func(bidID types.BidID) interface{}
}

type leaseFilterModifier struct {
	fieldName string
	f         func(leaseID types.LeaseID, filter types.LeaseFilters) types.LeaseFilters
	getField  func(leaseID types.LeaseID) interface{}
}

func TestGRPCQueryOrdersWithFilter(t *testing.T) {
	suite := setupTest(t)

	// creating orders with different states
	orderA, _ := createOrder(t, suite.ctx, suite.keeper)
	orderB, _ := createOrder(t, suite.ctx, suite.keeper)
	orderC, _ := createOrder(t, suite.ctx, suite.keeper)

	orders := []types.OrderID{
		orderA.GetOrderID(),
		orderB.GetOrderID(),
		orderC.GetOrderID(),
	}

	modifiers := []orderFilterModifier{
		{
			"owner",
			func(orderID types.OrderID, filter types.OrderFilters) types.OrderFilters {
				filter.Owner = orderID.GetOwner()
				return filter
			},
			func(orderID types.OrderID) interface{} {
				return orderID.Owner
			},
		},
		{
			"dseq",
			func(orderID types.OrderID, filter types.OrderFilters) types.OrderFilters {
				filter.DSeq = orderID.DSeq
				return filter
			},
			func(orderID types.OrderID) interface{} {
				return orderID.DSeq
			},
		},
		{
			"gseq",
			func(orderID types.OrderID, filter types.OrderFilters) types.OrderFilters {
				filter.GSeq = orderID.GSeq
				return filter
			},
			func(orderID types.OrderID) interface{} {
				return orderID.GSeq
			},
		},
		{
			"oseq",
			func(orderID types.OrderID, filter types.OrderFilters) types.OrderFilters {
				filter.OSeq = orderID.OSeq
				return filter
			},
			func(orderID types.OrderID) interface{} {
				return orderID.OSeq
			},
		},
	}

	ctx := sdk.WrapSDKContext(suite.ctx)

	for _, orderID := range orders {
		for _, m := range modifiers {
			req := &types.QueryOrdersRequest{
				Filters: m.f(orderID, types.OrderFilters{}),
			}

			res, err := suite.queryClient.Orders(ctx, req)

			require.NoError(t, err, "testing %v", m.fieldName)
			require.NotNil(t, res, "testing %v", m.fieldName)
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Orders), 1, "testing %v", m.fieldName)

			for _, order := range res.Orders {
				resultOrderID := order.GetOrderID()
				require.Equal(t, m.getField(orderID), m.getField(resultOrderID), "testing %v", m.fieldName)
			}
		}
	}

	limit := int(math.Pow(2, float64(len(modifiers))))

	// Use an order ID that matches absolutely nothing in any field
	bogusOrderID := types.OrderID{
		Owner: testutil.AccAddress(t).String(),
		DSeq:  9999999,
		GSeq:  8888888,
		OSeq:  7777777,
	}
	for i := 0; i != limit; i++ {
		modifiersToUse := make([]bool, len(modifiers))

		for j := 0; j != len(modifiers); j++ {
			mask := int(math.Pow(2, float64(j)))
			modifiersToUse[j] = (mask & i) != 0
		}

		for _, orderID := range orders {
			filter := types.OrderFilters{}
			msg := strings.Builder{}
			msg.WriteString("testing filtering on: ")
			for k, useModifier := range modifiersToUse {
				if !useModifier {
					continue
				}
				modifier := modifiers[k]
				filter = modifier.f(orderID, filter)
				msg.WriteString(modifier.fieldName)
				msg.WriteString(", ")
			}

			req := &types.QueryOrdersRequest{
				Filters: filter,
			}

			res, err := suite.queryClient.Orders(ctx, req)

			require.NoError(t, err, msg.String())
			require.NotNil(t, res, msg.String())
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Orders), 1, msg.String())

			for _, order := range res.Orders {
				resultOrderID := order.GetOrderID()
				for k, useModifier := range modifiersToUse {
					if !useModifier {
						continue
					}
					m := modifiers[k]
					require.Equal(t, m.getField(orderID), m.getField(resultOrderID), "testing %v", m.fieldName)
				}
			}
		}

		filter := types.OrderFilters{}
		msg := strings.Builder{}
		msg.WriteString("testing filtering on (using non matching ID): ")
		for k, useModifier := range modifiersToUse {
			if !useModifier {
				continue
			}
			modifier := modifiers[k]
			filter = modifier.f(bogusOrderID, filter)
			msg.WriteString(modifier.fieldName)
			msg.WriteString(", ")
		}

		req := &types.QueryOrdersRequest{
			Filters: filter,
		}

		res, err := suite.queryClient.Orders(ctx, req)

		require.NoError(t, err, msg.String())
		require.NotNil(t, res, msg.String())
		expected := 0
		if i == 0 {
			expected = len(orders)
		}
		require.Len(t, res.Orders, expected, msg.String())
	}

	for _, orderID := range orders {
		// Query by owner
		req := &types.QueryOrdersRequest{
			Filters: types.OrderFilters{
				Owner: orderID.Owner,
			},
		}

		res, err := suite.queryClient.Orders(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// Just 1 result
		require.Len(t, res.Orders, 1)
		orderResult := res.Orders[0]
		require.Equal(t, orderID, orderResult.GetOrderID())

		// Query with valid DSeq
		req = &types.QueryOrdersRequest{
			Filters: types.OrderFilters{
				Owner: orderID.Owner,
				DSeq:  orderID.DSeq,
			},
		}

		res, err = suite.queryClient.Orders(ctx, req)

		// Expect the same match
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Orders, 1)
		orderResult = res.Orders[0]
		require.Equal(t, orderID, orderResult.GetOrderID())

		// Query with a bogus DSeq
		req = &types.QueryOrdersRequest{
			Filters: types.OrderFilters{
				Owner: orderID.Owner,
				DSeq:  orderID.DSeq + 1,
			},
		}

		res, err = suite.queryClient.Orders(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// expect nothing matches
		require.Len(t, res.Orders, 0)
	}
}

func TestGRPCQueryBidsWithFilter(t *testing.T) {
	suite := setupTest(t)

	// creating bids with different states
	bidA, _ := createBid(t, suite.TestSuite)
	bidB, _ := createBid(t, suite.TestSuite)
	bidC, _ := createBid(t, suite.TestSuite)

	bids := []types.BidID{
		bidA.GetBidID(),
		bidB.GetBidID(),
		bidC.GetBidID(),
	}

	modifiers := []bidFilterModifier{
		{
			"owner",
			func(bidID types.BidID, filter types.BidFilters) types.BidFilters {
				filter.Owner = bidID.GetOwner()
				return filter
			},
			func(bidID types.BidID) interface{} {
				return bidID.Owner
			},
		},
		{
			"dseq",
			func(bidID types.BidID, filter types.BidFilters) types.BidFilters {
				filter.DSeq = bidID.DSeq
				return filter
			},
			func(bidID types.BidID) interface{} {
				return bidID.DSeq
			},
		},
		{
			"gseq",
			func(bidID types.BidID, filter types.BidFilters) types.BidFilters {
				filter.GSeq = bidID.GSeq
				return filter
			},
			func(bidID types.BidID) interface{} {
				return bidID.GSeq
			},
		},
		{
			"oseq",
			func(bidID types.BidID, filter types.BidFilters) types.BidFilters {
				filter.OSeq = bidID.OSeq
				return filter
			},
			func(bidID types.BidID) interface{} {
				return bidID.OSeq
			},
		},
		{
			"provider",
			func(bidID types.BidID, filter types.BidFilters) types.BidFilters {
				filter.Provider = bidID.Provider
				return filter
			},
			func(bidID types.BidID) interface{} {
				return bidID.Provider
			},
		},
	}

	ctx := sdk.WrapSDKContext(suite.ctx)

	for _, bidID := range bids {
		for _, m := range modifiers {
			req := &types.QueryBidsRequest{
				Filters: m.f(bidID, types.BidFilters{}),
			}

			res, err := suite.queryClient.Bids(ctx, req)

			require.NoError(t, err, "testing %v", m.fieldName)
			require.NotNil(t, res, "testing %v", m.fieldName)
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Bids), 1, "testing %v", m.fieldName)

			for _, bid := range res.Bids {
				resultBidID := bid.GetBid().BidID
				require.Equal(t, m.getField(bidID), m.getField(resultBidID), "testing %v", m.fieldName)
			}
		}
	}

	limit := int(math.Pow(2, float64(len(modifiers))))

	// Use an order ID that matches absolutely nothing in any field
	bogusBidID := types.BidID{
		Owner:    testutil.AccAddress(t).String(),
		DSeq:     9999999,
		GSeq:     8888888,
		OSeq:     7777777,
		Provider: testutil.AccAddress(t).String(),
	}
	for i := 0; i != limit; i++ {
		modifiersToUse := make([]bool, len(modifiers))

		for j := 0; j != len(modifiers); j++ {
			mask := int(math.Pow(2, float64(j)))
			modifiersToUse[j] = (mask & i) != 0
		}

		for _, bidID := range bids {
			filter := types.BidFilters{}
			msg := strings.Builder{}
			msg.WriteString("testing filtering on: ")
			for k, useModifier := range modifiersToUse {
				if !useModifier {
					continue
				}
				modifier := modifiers[k]
				filter = modifier.f(bidID, filter)
				msg.WriteString(modifier.fieldName)
				msg.WriteString(", ")
			}

			req := &types.QueryBidsRequest{
				Filters: filter,
			}

			res, err := suite.queryClient.Bids(ctx, req)

			require.NoError(t, err, msg.String())
			require.NotNil(t, res, msg.String())
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Bids), 1, msg.String())

			for _, bid := range res.Bids {
				resultBidID := bid.GetBid().BidID
				for k, useModifier := range modifiersToUse {
					if !useModifier {
						continue
					}
					m := modifiers[k]
					require.Equal(t, m.getField(bidID), m.getField(resultBidID), "testing %v", m.fieldName)
				}
			}
		}

		filter := types.BidFilters{}
		msg := strings.Builder{}
		msg.WriteString("testing filtering on (using non matching ID): ")
		for k, useModifier := range modifiersToUse {
			if !useModifier {
				continue
			}
			modifier := modifiers[k]
			filter = modifier.f(bogusBidID, filter)
			msg.WriteString(modifier.fieldName)
			msg.WriteString(", ")
		}

		req := &types.QueryBidsRequest{
			Filters: filter,
		}

		res, err := suite.queryClient.Bids(ctx, req)

		require.NoError(t, err, msg.String())
		require.NotNil(t, res, msg.String())
		expected := 0
		if i == 0 {
			expected = len(bids)
		}
		require.Len(t, res.Bids, expected, msg.String())
	}

	for _, bidID := range bids {
		// Query by owner
		req := &types.QueryBidsRequest{
			Filters: types.BidFilters{
				Owner: bidID.Owner,
			},
		}

		res, err := suite.queryClient.Bids(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// Just 1 result
		require.Len(t, res.Bids, 1)
		bidResult := res.Bids[0]
		require.Equal(t, bidID, bidResult.GetBid().BidID)

		// Query with valid DSeq
		req = &types.QueryBidsRequest{
			Filters: types.BidFilters{
				Owner: bidID.Owner,
				DSeq:  bidID.DSeq,
			},
		}

		res, err = suite.queryClient.Bids(ctx, req)

		// Expect the same match
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Bids, 1)
		bidResult = res.Bids[0]
		require.Equal(t, bidID, bidResult.GetBid().BidID)

		// Query with a bogus DSeq
		req = &types.QueryBidsRequest{
			Filters: types.BidFilters{
				Owner: bidID.Owner,
				DSeq:  bidID.DSeq + 1,
			},
		}

		res, err = suite.queryClient.Bids(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// expect nothing matches
		require.Len(t, res.Bids, 0)
	}
}

func TestGRPCQueryLeasesWithFilter(t *testing.T) {
	suite := setupTest(t)

	// creating leases with different states
	leaseA := createLease(t, suite.TestSuite)
	leaseB := createLease(t, suite.TestSuite)
	leaseC := createLease(t, suite.TestSuite)

	leases := []types.LeaseID{
		leaseA,
		leaseB,
		leaseC,
	}

	modifiers := []leaseFilterModifier{
		{
			"owner",
			func(leaseID types.LeaseID, filter types.LeaseFilters) types.LeaseFilters {
				filter.Owner = leaseID.GetOwner()
				return filter
			},
			func(leaseID types.LeaseID) interface{} {
				return leaseID.Owner
			},
		},
		{
			"dseq",
			func(leaseID types.LeaseID, filter types.LeaseFilters) types.LeaseFilters {
				filter.DSeq = leaseID.DSeq
				return filter
			},
			func(leaseID types.LeaseID) interface{} {
				return leaseID.DSeq
			},
		},
		{
			"gseq",
			func(leaseID types.LeaseID, filter types.LeaseFilters) types.LeaseFilters {
				filter.GSeq = leaseID.GSeq
				return filter
			},
			func(leaseID types.LeaseID) interface{} {
				return leaseID.GSeq
			},
		},
		{
			"oseq",
			func(leaseID types.LeaseID, filter types.LeaseFilters) types.LeaseFilters {
				filter.OSeq = leaseID.OSeq
				return filter
			},
			func(leaseID types.LeaseID) interface{} {
				return leaseID.OSeq
			},
		},
		{
			"provider",
			func(leaseID types.LeaseID, filter types.LeaseFilters) types.LeaseFilters {
				filter.Provider = leaseID.Provider
				return filter
			},
			func(leaseID types.LeaseID) interface{} {
				return leaseID.Provider
			},
		},
	}

	ctx := sdk.WrapSDKContext(suite.ctx)

	for _, leaseID := range leases {
		for _, m := range modifiers {
			req := &types.QueryLeasesRequest{
				Filters: m.f(leaseID, types.LeaseFilters{}),
			}

			res, err := suite.queryClient.Leases(ctx, req)

			require.NoError(t, err, "testing %v", m.fieldName)
			require.NotNil(t, res, "testing %v", m.fieldName)
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Leases), 1, "testing %v", m.fieldName)

			for _, lease := range res.Leases {
				resultLeaseID := lease.Lease.GetLeaseID()
				require.Equal(t, m.getField(leaseID), m.getField(resultLeaseID), "testing %v", m.fieldName)
			}
		}
	}

	limit := int(math.Pow(2, float64(len(modifiers))))

	// Use an order ID that matches absolutely nothing in any field
	bogusBidID := types.LeaseID{
		Owner:    testutil.AccAddress(t).String(),
		DSeq:     9999999,
		GSeq:     8888888,
		OSeq:     7777777,
		Provider: testutil.AccAddress(t).String(),
	}
	for i := 0; i != limit; i++ {
		modifiersToUse := make([]bool, len(modifiers))

		for j := 0; j != len(modifiers); j++ {
			mask := int(math.Pow(2, float64(j)))
			modifiersToUse[j] = (mask & i) != 0
		}

		for _, leaseID := range leases {
			filter := types.LeaseFilters{}
			msg := strings.Builder{}
			msg.WriteString("testing filtering on: ")
			for k, useModifier := range modifiersToUse {
				if !useModifier {
					continue
				}
				modifier := modifiers[k]
				filter = modifier.f(leaseID, filter)
				msg.WriteString(modifier.fieldName)
				msg.WriteString(", ")
			}

			req := &types.QueryLeasesRequest{
				Filters: filter,
			}

			res, err := suite.queryClient.Leases(ctx, req)

			require.NoError(t, err, msg.String())
			require.NotNil(t, res, msg.String())
			// At least 1 result
			require.GreaterOrEqual(t, len(res.Leases), 1, msg.String())

			for _, lease := range res.Leases {
				resultLeaseID := lease.GetLease().LeaseID
				for k, useModifier := range modifiersToUse {
					if !useModifier {
						continue
					}
					m := modifiers[k]
					require.Equal(t, m.getField(leaseID), m.getField(resultLeaseID), "testing %v", m.fieldName)
				}
			}
		}

		filter := types.LeaseFilters{}
		msg := strings.Builder{}
		msg.WriteString("testing filtering on (using non matching ID): ")
		for k, useModifier := range modifiersToUse {
			if !useModifier {
				continue
			}
			modifier := modifiers[k]
			filter = modifier.f(bogusBidID, filter)
			msg.WriteString(modifier.fieldName)
			msg.WriteString(", ")
		}

		req := &types.QueryLeasesRequest{
			Filters: filter,
		}

		res, err := suite.queryClient.Leases(ctx, req)

		require.NoError(t, err, msg.String())
		require.NotNil(t, res, msg.String())
		expected := 0
		if i == 0 {
			expected = len(leases)
		}
		require.Len(t, res.Leases, expected, msg.String())
	}

	for _, leaseID := range leases {
		// Query by owner
		req := &types.QueryLeasesRequest{
			Filters: types.LeaseFilters{
				Owner: leaseID.Owner,
			},
		}

		res, err := suite.queryClient.Leases(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// Just 1 result
		require.Len(t, res.Leases, 1)
		leaseResult := res.Leases[0]
		require.Equal(t, leaseID, leaseResult.GetLease().LeaseID)

		// Query with valid DSeq
		req = &types.QueryLeasesRequest{
			Filters: types.LeaseFilters{
				Owner: leaseID.Owner,
				DSeq:  leaseID.DSeq,
			},
		}

		res, err = suite.queryClient.Leases(ctx, req)

		// Expect the same match
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Leases, 1)
		leaseResult = res.Leases[0]
		require.Equal(t, leaseID, leaseResult.GetLease().LeaseID)

		// Query with a bogus DSeq
		req = &types.QueryLeasesRequest{
			Filters: types.LeaseFilters{
				Owner: leaseID.Owner,
				DSeq:  leaseID.DSeq + 1,
			},
		}

		res, err = suite.queryClient.Leases(ctx, req)

		require.NoError(t, err)
		require.NotNil(t, res)
		// expect nothing matches
		require.Len(t, res.Leases, 0)
	}
}

func TestGRPCQueryBid(t *testing.T) {
	suite := setupTest(t)

	// creating bid
	bid, _ := createBid(t, suite.TestSuite)

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
	_, _ = createBid(t, suite.TestSuite)
	bid2, _ := createBid(t, suite.TestSuite)
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
						State:    types.BidLost.String(),
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
	leaseID := createLease(t, suite.TestSuite)
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
	leaseID := createLease(t, suite.TestSuite)
	_, ok := suite.keeper.GetLease(suite.ctx, leaseID)
	require.True(t, ok)

	leaseID2 := createLease(t, suite.TestSuite)
	lease2, ok := suite.keeper.GetLease(suite.ctx, leaseID2)
	require.True(t, ok)
	suite.keeper.OnLeaseClosed(suite.ctx, lease2, types.LeaseClosed)

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
						State:    types.LeaseClosed.String(),
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

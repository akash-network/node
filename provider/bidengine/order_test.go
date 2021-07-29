package bidengine

import (
	"context"
	"errors"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/sdkutil"

	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/stretchr/testify/require"

	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	atypes "github.com/ovrclk/akash/types"
	audittypes "github.com/ovrclk/akash/x/audit/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"

	broadcastmocks "github.com/ovrclk/akash/client/broadcaster/mocks"
	clientmocks "github.com/ovrclk/akash/client/mocks"
	clustermocks "github.com/ovrclk/akash/provider/cluster/mocks"
)

type orderTestScaffold struct {
	orderID      mtypes.OrderID
	groupID      dtypes.GroupID
	testBus      pubsub.Bus
	testAddr     sdk.AccAddress
	deploymentID dtypes.DeploymentID
	bidID        *mtypes.BidID

	queryClient *clientmocks.QueryClient
	client      *clientmocks.Client
	txClient    *broadcastmocks.Client
	cluster     *clustermocks.Cluster

	broadcasts        chan sdk.Msg
	reserveCallNotify chan int
}

func makeMocks(s *orderTestScaffold) {

	groupResult := &dtypes.QueryGroupResponse{}
	groupResult.Group.GroupSpec.Name = "testGroupName"
	groupResult.Group.GroupSpec.Resources = make([]dtypes.Resource, 1)

	cpu := atypes.CPU{}
	cpu.Units = atypes.NewResourceValue(uint64(dtypes.GetValidationConfig().MinUnitCPU))

	memory := atypes.Memory{}
	memory.Quantity = atypes.NewResourceValue(dtypes.GetValidationConfig().MinUnitMemory)

	storage := atypes.Storage{}
	storage.Quantity = atypes.NewResourceValue(dtypes.GetValidationConfig().MinUnitStorage)

	clusterResources := atypes.ResourceUnits{
		CPU:     &cpu,
		Memory:  &memory,
		Storage: &storage,
	}
	price := sdk.NewInt64Coin(testutil.CoinDenom, 23)
	resource := dtypes.Resource{
		Resources: clusterResources,
		Count:     2,
		Price:     price,
	}

	groupResult.Group.GroupSpec.Resources[0] = resource

	queryClientMock := &clientmocks.QueryClient{}
	queryClientMock.On("Group", mock.Anything, mock.Anything).Return(groupResult, nil)

	queryClientMock.On("Orders", mock.Anything, mock.Anything).Return(&mtypes.QueryOrdersResponse{}, nil)

	txClientMock := &broadcastmocks.Client{}
	s.broadcasts = make(chan sdk.Msg, 1)
	txClientMock.On("Broadcast", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		s.broadcasts <- args.Get(1).(sdk.Msg)
	}).Return(nil)

	clientMock := &clientmocks.Client{}
	clientMock.On("Query").Return(queryClientMock)
	clientMock.On("Tx").Return(txClientMock)

	s.client = clientMock
	s.queryClient = queryClientMock
	s.txClient = txClientMock

	mockReservation := &clustermocks.Reservation{}
	mockReservation.On("OrderID").Return(s.orderID)
	mockReservation.On("Resources").Return(groupResult.Group)

	s.cluster = &clustermocks.Cluster{}
	s.reserveCallNotify = make(chan int, 1)
	s.cluster.On("Reserve", s.orderID, &(groupResult.Group)).Run(func(args mock.Arguments) {
		s.reserveCallNotify <- 0
		time.Sleep(time.Second) // add a delay before returning response, to test race conditions
	}).Return(mockReservation, nil)

	s.cluster.On("Unreserve", s.orderID, mock.Anything).Return(nil)

}

type nullProviderAttrSignatureService struct{}

func (nullProviderAttrSignatureService) GetAuditorAttributeSignatures(auditor string) ([]audittypes.Provider, error) {
	return nil, nil // Return no attributes & no error
}

func makeOrderForTest(t *testing.T, checkForExistingBid bool, pricing BidPricingStrategy, callerConfig *Config) (*order, orderTestScaffold, <-chan int) {
	if pricing == nil {
		var err error
		pricing, err = MakeRandomRangePricing()
		require.NoError(t, err)
		require.NotNil(t, pricing)
	}

	var scaffold orderTestScaffold
	scaffold.deploymentID = testutil.DeploymentID(t)

	scaffold.groupID = dtypes.MakeGroupID(scaffold.deploymentID, 2)

	scaffold.orderID = mtypes.MakeOrderID(scaffold.groupID, 1356326)

	myLog := testutil.Logger(t)

	makeMocks(&scaffold)

	scaffold.testAddr = testutil.AccAddress(t)

	myProvider := &ptypes.Provider{
		Owner:      scaffold.testAddr.String(),
		HostURI:    "",
		Attributes: nil,
	}
	mySession := session.New(myLog, scaffold.client, myProvider)

	scaffold.testBus = pubsub.NewBus()
	var cfg Config
	if callerConfig != nil {
		cfg = *callerConfig // Copy values from caller
	}
	// Overwrite some with stuff built in this function
	cfg.PricingStrategy = pricing
	cfg.Deposit = mtypes.DefaultBidMinDeposit

	myService, err := NewService(context.Background(), mySession, scaffold.cluster, scaffold.testBus, cfg)
	require.NoError(t, err)
	require.NotNil(t, myService)

	serviceCast := myService.(*service)

	if checkForExistingBid {
		bidID := mtypes.MakeBidID(scaffold.orderID, mySession.Provider().Address())
		scaffold.bidID = &bidID
		queryBidRequest := &mtypes.QueryBidRequest{
			ID: bidID,
		}
		response := &mtypes.QueryBidResponse{
			Bid: mtypes.Bid{
				BidID: bidID,
				State: mtypes.BidOpen,
				Price: sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(int64(testutil.RandRangeInt(100, 1000)))),
			},
		}
		scaffold.queryClient.On("Bid", mock.Anything, queryBidRequest).Return(response, nil)
	}

	reservationFulfilledNotify := make(chan int, 1)
	order, err := newOrderInternal(serviceCast, scaffold.orderID, cfg, nullProviderAttrSignatureService{}, checkForExistingBid, reservationFulfilledNotify)

	require.NoError(t, err)
	require.NotNil(t, order)

	return order, scaffold, reservationFulfilledNotify
}

func Test_BidOrderAndUnreserve(t *testing.T) {
	order, scaffold, _ := makeOrderForTest(t, false, nil, nil)

	broadcast := testutil.ChannelWaitForValue(t, scaffold.broadcasts)
	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	createBidMsg, ok := broadcast.(*mtypes.MsgCreateBid)
	require.True(t, ok)

	require.Equal(t, createBidMsg.Order, scaffold.orderID)

	priceDenom := createBidMsg.Price.Denom
	require.Equal(t, testutil.CoinDenom, priceDenom)
	priceAmount := createBidMsg.Price.Amount.Int64()

	require.GreaterOrEqual(t, priceAmount, int64(1))
	require.Less(t, priceAmount, int64(100))

	// After the broadcast call shut the thing down
	// and then check what was broadcast
	order.lc.Shutdown(nil)

	// Should have called unreserve once, nothing happened after the bid
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)
}

func Test_BidOrderAndUnreserveOnTimeout(t *testing.T) {
	order, scaffold, _ := makeOrderForTest(t, false, nil, &Config{
		BidTimeout: 5 * time.Second,
	})

	broadcast := testutil.ChannelWaitForValue(t, scaffold.broadcasts)
	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	createBidMsg, ok := broadcast.(*mtypes.MsgCreateBid)
	require.True(t, ok)

	require.Equal(t, createBidMsg.Order, scaffold.orderID)

	priceDenom := createBidMsg.Price.Denom
	require.Equal(t, testutil.CoinDenom, priceDenom)
	priceAmount := createBidMsg.Price.Amount.Int64()

	require.GreaterOrEqual(t, priceAmount, int64(1))
	require.Less(t, priceAmount, int64(100))

	// After the broadcast call the timeout should take effect
	// and then close the bid, unreserving capacity in the process
	broadcast = testutil.ChannelWaitForValue(t, scaffold.broadcasts)
	_, ok = broadcast.(*mtypes.MsgCloseBid)
	require.True(t, ok)

	// After the broadcast call shut down happens automatically
	order.lc.Shutdown(nil)
	select {
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting on shutdown")
	case <-order.lc.Done():
		break
	}

	// Should have called unreserve once
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)
}

func Test_BidOrderPriceTooHigh(t *testing.T) {
	pricing := testBidPricingStrategy(9999999999)
	order, scaffold, _ := makeOrderForTest(t, false, pricing, nil)

	select {
	case <-order.lc.Done(): // Should stop on its own

	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting in test")
	}
	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	select {
	case <-scaffold.broadcasts:
		t.Fatal("should not have broadcast anything")
	default:
	}

	// Should have called unreserve once, nothing happened after the bid
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)

}

func Test_BidOrderAndThenClosedUnreserve(t *testing.T) {
	order, scaffold, _ := makeOrderForTest(t, false, nil, nil)

	testutil.ChannelWaitForValue(t, scaffold.broadcasts)
	// Should have called reserve once at this point
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	ev := mtypes.EventOrderClosed{
		Context: sdkutil.BaseModuleEvent{},
		ID:      scaffold.orderID,
	}
	err := scaffold.testBus.Publish(ev)
	require.NoError(t, err)

	// Wait for this to complete. An order close event has happened so it stops
	// on its own
	<-order.lc.Done()

	// Should have called unreserve once
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)
}

func Test_OrderCloseBeforeReserveReturn(t *testing.T) {
	order, scaffold, reservationFulfilledNotify := makeOrderForTest(t, false, nil, nil)

	testutil.ChannelWaitForValue(t, scaffold.reserveCallNotify)
	// Should have called reserve once at this point
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	// reservationFulfilledNotify channel shouldn't have got any value yet because the Reserve call
	// returns after a delay of one second
	select {
	case <-reservationFulfilledNotify:
		t.Fatal("reservation shouldn't have been fulfilled")
	default:
	}

	// close the order before Reserve call returns
	ev := mtypes.EventOrderClosed{
		Context: sdkutil.BaseModuleEvent{},
		ID:      scaffold.orderID,
	}
	err := scaffold.testBus.Publish(ev)
	require.NoError(t, err)

	// reservationFulfilledNotify channel can't get any value now because the order close event
	// should take priority
	select {
	case <-reservationFulfilledNotify:
		t.Fatal("reservation shouldn't have been fulfilled")
	default:
	}

	// Wait for this to complete. An order close event has happened so it stops
	// on its own
	<-order.lc.Done()

	// Should have called unreserve once
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)
}

func Test_BidOrderAndThenLeaseCreated(t *testing.T) {
	order, scaffold, _ := makeOrderForTest(t, false, nil, nil)

	// Wait for first broadcast
	broadcast := testutil.ChannelWaitForValue(t, scaffold.broadcasts)

	createBidMsg, ok := broadcast.(*mtypes.MsgCreateBid)
	require.True(t, ok)
	require.Equal(t, createBidMsg.Order, scaffold.orderID)
	priceDenom := createBidMsg.Price.Denom
	require.Equal(t, testutil.CoinDenom, priceDenom)
	priceAmount := createBidMsg.Price.Amount.Int64()

	require.GreaterOrEqual(t, priceAmount, int64(1))
	require.Less(t, priceAmount, int64(100))

	leaseID := mtypes.MakeLeaseID(mtypes.MakeBidID(scaffold.orderID, scaffold.testAddr))

	ev := mtypes.EventLeaseCreated{
		Context: sdkutil.BaseModuleEvent{},
		ID:      leaseID,
		Price:   testutil.AkashCoin(t, 1),
	}

	require.Equal(t, order.orderID.GroupID(), ev.ID.GroupID())

	err := scaffold.testBus.Publish(ev)
	require.NoError(t, err)

	// Wait for this to complete. The lease has been created so it
	// stops on it own
	<-order.lc.Done()

	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	// Should not have called unreserve
	scaffold.cluster.AssertNotCalled(t, "Unreserve", mock.Anything, mock.Anything)
}

func Test_BidOrderAndThenLeaseCreatedForDifferentDeployment(t *testing.T) {

	order, scaffold, _ := makeOrderForTest(t, false, nil, nil)

	// Wait for first broadcast
	broadcast := testutil.ChannelWaitForValue(t, scaffold.broadcasts)

	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	createBidMsg, ok := broadcast.(*mtypes.MsgCreateBid)
	require.True(t, ok)
	require.Equal(t, createBidMsg.Order, scaffold.orderID)

	otherOrderID := scaffold.orderID
	otherOrderID.GSeq++
	leaseID := mtypes.MakeLeaseID(mtypes.MakeBidID(otherOrderID, scaffold.testAddr))

	ev := mtypes.EventLeaseCreated{
		Context: sdkutil.BaseModuleEvent{},
		ID:      leaseID,
		Price:   testutil.AkashCoin(t, 1),
	}

	subscriber, err := scaffold.testBus.Subscribe()
	require.NoError(t, err)
	err = scaffold.testBus.Publish(ev)
	require.NoError(t, err)

	// Wait for the event to be published
	testutil.ChannelWaitForValue(t, subscriber.Events())

	// Should not have called unreserve yet
	scaffold.cluster.AssertNotCalled(t, "Unreserve", mock.Anything, mock.Anything)

	// Shutdown after the message has been published
	order.lc.Shutdown(nil)

	// Should have called unreserve
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)

	// The last call should be a broadcast to close the bid
	txCalls := scaffold.txClient.Calls
	require.NotEqual(t, 0, len(txCalls))
	lastBroadcast := txCalls[len(txCalls)-1]
	require.Equal(t, "Broadcast", lastBroadcast.Method)
	msg := lastBroadcast.Arguments[1]
	closeBidMsg, ok := msg.(*mtypes.MsgCloseBid)
	require.True(t, ok)
	require.NotNil(t, closeBidMsg)
	expectedBidID := mtypes.MakeBidID(order.orderID, scaffold.testAddr)
	require.Equal(t, closeBidMsg.BidID, expectedBidID)
}

func Test_ShouldNotBidWhenAlreadySet(t *testing.T) {

	order, scaffold, reservationFulfilledNotify := makeOrderForTest(t, true, nil, nil)

	// Wait for a reserve call
	testutil.ChannelWaitForValue(t, scaffold.reserveCallNotify)

	// Should have queried for the bid
	queryCalls := scaffold.queryClient.Calls

	var lastBid mock.Call

	for _, call := range queryCalls {
		if call.Method == "Bid" {
			lastBid = call
		}
	}
	require.Equal(t, "Bid", lastBid.Method)
	query, ok := lastBid.Arguments[1].(*mtypes.QueryBidRequest)
	require.True(t, ok)
	require.Equal(t, *scaffold.bidID, query.ID)

	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	// Wait for the reservation to be processed
	testutil.ChannelWaitForValue(t, reservationFulfilledNotify)

	// Close the order
	ev := mtypes.EventOrderClosed{
		Context: sdkutil.BaseModuleEvent{},
		ID:      scaffold.orderID,
	}

	err := scaffold.testBus.Publish(ev)
	require.NoError(t, err)

	// Wait for it to stop
	<-order.lc.Done()

	// Should have called unreserve during shutdown
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)

	var broadcast sdk.Msg
	select {
	case broadcast = <-scaffold.broadcasts:
	default:
	}
	// Should have broadcast
	require.NotNil(t, broadcast)
	closeBid, ok := broadcast.(*mtypes.MsgCloseBid)
	require.True(t, ok)
	require.NotNil(t, closeBid)

	require.Equal(t, closeBid.BidID, *scaffold.bidID)
}

func Test_ShouldRecognizeLeaseCreatedIfBiddingIsSkipped(t *testing.T) {
	order, scaffold, _ := makeOrderForTest(t, true, nil, nil)

	// Wait for a reserve call
	testutil.ChannelWaitForValue(t, scaffold.reserveCallNotify)

	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	// Should not have called unreserve
	scaffold.cluster.AssertNotCalled(t, "Unreserve", mock.Anything, mock.Anything)

	leaseID := mtypes.MakeLeaseID(mtypes.MakeBidID(scaffold.orderID, scaffold.testAddr))

	ev := mtypes.EventLeaseCreated{
		Context: sdkutil.BaseModuleEvent{},
		ID:      leaseID,
		Price:   testutil.AkashCoin(t, 1),
	}

	err := scaffold.testBus.Publish(ev)
	require.NoError(t, err)

	// Wait for it to stop
	<-order.lc.Done()

	// Should not have called unreserve during shutdown
	scaffold.cluster.AssertNotCalled(t, "Unreserve", mock.Anything, mock.Anything)

	var broadcast sdk.Msg

	select {
	case broadcast = <-scaffold.broadcasts:
	default:
	}
	// Should never have broadcast
	require.Nil(t, broadcast)
}

type testBidPricingStrategy int64

func (tbps testBidPricingStrategy) CalculatePrice(_ context.Context, _ string, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
	return sdk.NewInt64Coin(testutil.CoinDenom, int64(tbps)), nil
}

func Test_BidOrderUsesBidPricingStrategy(t *testing.T) {
	expectedBid := int64(37)
	// Create a test strategy that gives a fixed price
	pricing := testBidPricingStrategy(expectedBid)
	order, scaffold, _ := makeOrderForTest(t, false, pricing, nil)

	broadcast := testutil.ChannelWaitForValue(t, scaffold.broadcasts)

	createBidMsg, ok := broadcast.(*mtypes.MsgCreateBid)
	require.True(t, ok)
	require.Equal(t, createBidMsg.Order, scaffold.orderID)

	priceDenom := createBidMsg.Price.Denom
	require.Equal(t, testutil.CoinDenom, priceDenom)
	priceAmount := createBidMsg.Price.Amount.Int64()

	require.Equal(t, priceAmount, expectedBid)

	// After the broadcast call shut the thing down
	// and then check what was broadcast
	order.lc.Shutdown(nil)

	// Should have called unreserve once, nothing happened after the bid
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)
}

type alwaysFailsBidPricingStrategy struct {
	failure error
}

func (afbps alwaysFailsBidPricingStrategy) CalculatePrice(_ context.Context, _ string, gspec *dtypes.GroupSpec) (sdk.Coin, error) {
	return sdk.Coin{}, afbps.failure
}

var errBidPricingAlwaysFails = errors.New("bid pricing fail in test")

func Test_BidOrderFailsAndAborts(t *testing.T) {
	// Create a test strategy that gives a fixed price
	pricing := alwaysFailsBidPricingStrategy{failure: errBidPricingAlwaysFails}
	order, scaffold, _ := makeOrderForTest(t, false, pricing, nil)

	<-order.lc.Done() // Stops whenever the bid pricing is called and returns an errro

	// Should have called reserve once
	scaffold.cluster.AssertCalled(t, "Reserve", scaffold.orderID, mock.Anything)

	var broadcast sdk.Msg

	select {
	case broadcast = <-scaffold.broadcasts:
	default:
	}
	// Should never have broadcast since bid pricing failed
	require.Nil(t, broadcast)

	// Should have called unreserve once, nothing happened after the bid
	scaffold.cluster.AssertCalled(t, "Unreserve", scaffold.orderID, mock.Anything)
}

// TODO - add test failing the call to Broadcast on TxClient and
// and then confirm that the reservation is cancelled

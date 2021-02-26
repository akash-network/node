package provider

import (
	"context"
	"github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	cosmosMock "github.com/ovrclk/akash/testutil/cosmos_mock"
)

type scaffold struct {
	testAddr    sdk.AccAddress
	testBus     pubsub.Bus
	ctx         context.Context
	cancel      context.CancelFunc
	queryClient *cosmosMock.QueryClient
	bc          *balanceChecker
}

func (s *scaffold) start() {
	go s.bc.lc.WatchContext(s.ctx)
	go s.bc.run()
}

func balanceCheckerForTest(t *testing.T, balance int64) (*scaffold, *balanceChecker) {
	s := &scaffold{}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	myLog := testutil.Logger(t)

	s.testAddr = testutil.AccAddress(t)

	queryClient := &cosmosMock.QueryClient{}
	query := bankTypes.NewQueryBalanceRequest(s.testAddr, testutil.CoinDenom)
	coin := sdk.NewCoin(testutil.CoinDenom, sdk.NewInt(balance))
	result := &bankTypes.QueryBalanceResponse{
		Balance: &coin,
	}
	queryClient.On("Balance", mock.Anything, query).Return(result, nil)

	myProvider := &ptypes.Provider{
		Owner:      s.testAddr.String(),
		HostURI:    "",
		Attributes: nil,
	}
	mySession := session.New(myLog, nil, myProvider)
	s.testBus = pubsub.NewBus()

	bc := &balanceChecker{
		session:         mySession,
		log:             myLog,
		lc:              lifecycle.New(),
		bus:             s.testBus,
		ownAddr:         s.testAddr,
		checkPeriod:     time.Millisecond * 100,
		bankQueryClient: queryClient,
	}

	s.queryClient = queryClient
	s.bc = bc

	return s, bc
}

func TestBalanceCheckerChecksBalance(t *testing.T) {
	testScaffold, bc := balanceCheckerForTest(t, 9999999999999)
	subscriber, err := testScaffold.testBus.Subscribe()
	require.NoError(t, err)

	firstEvent := make(chan pubsub.Event, 1)
	go func() {
		defer subscriber.Close()
		select {
		case ev := <-subscriber.Events():
			firstEvent <- ev
		case <-bc.lc.Done():

		}
	}()

	testScaffold.start()

	time.Sleep(bc.checkPeriod * 3)
	testScaffold.cancel()
	<-bc.lc.Done()

	firstCall := testScaffold.queryClient.Calls[0]
	require.Equal(t, "Balance", firstCall.Method)

	// Make sure no event is sent
	select {
	case <-firstEvent:
		t.Fatal("should not have an event to read")
	default:

	}
}

func TestBalanceCheckerStartsWithdrawal(t *testing.T) {
	testScaffold, bc := balanceCheckerForTest(t, 1)
	subscriber, err := testScaffold.testBus.Subscribe()
	require.NoError(t, err)

	firstEvent := make(chan pubsub.Event, 1)
	go func() {
		defer subscriber.Close()
		select {
		case ev := <-subscriber.Events():
			firstEvent <- ev
		case <-bc.lc.Done():

		}
	}()

	testScaffold.start()

	time.Sleep(bc.checkPeriod * 3)
	testScaffold.cancel()
	<-bc.lc.Done()

	firstCall := testScaffold.queryClient.Calls[0]
	require.Equal(t, "Balance", firstCall.Method)

	// Make sure the event is sent
	select {
	case ev := <-firstEvent:
		_, ok := ev.(event.LeaseWithdrawNow)
		require.True(t, ok)
	default:
		t.Fatal("should have an event to read")
	}
}

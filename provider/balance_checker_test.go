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
		HostURI:    "http://test.localhost:7443",
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
		bankQueryClient: queryClient,
		cfg: BalanceCheckerConfig{
			PollingPeriod:           time.Millisecond * 100, // TODO
			MinimumBalanceThreshold: 100,
			WithdrawalPeriod:        time.Hour * 24,
		},
	}

	s.queryClient = queryClient
	s.bc = bc

	return s, bc
}

func TestBalanceCheckerChecksBalance(t *testing.T) {
	testScaffold, bc := balanceCheckerForTest(t, 9999999999999)
	defer testScaffold.testBus.Close()
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

	time.Sleep(bc.cfg.PollingPeriod * 3)
	testScaffold.cancel()
	<-bc.lc.Done()

	testScaffold.queryClient.AssertExpectations(t)

	// Make sure no event is sent
	select {
	case <-firstEvent:
		t.Fatal("should not have an event to read")
	default:

	}
}

func TestBalanceCheckerStartsWithdrawal(t *testing.T) {
	testScaffold, bc := balanceCheckerForTest(t, 1)
	defer testScaffold.testBus.Close()
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

	// Make sure the event is sent
	select {
	case ev := <-firstEvent:
		_, ok := ev.(event.LeaseWithdrawNow)
		require.True(t, ok)
	case <-time.After(15 * time.Second):
		t.Fatal("should have an event to read")
	}

	time.Sleep(bc.cfg.PollingPeriod)
	testScaffold.cancel()

	select {
	case <-bc.lc.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for completion")
	}

	testScaffold.queryClient.AssertExpectations(t)
}

package provider

import (
	"context"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	tmrpc "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/testutil"
	netutil "github.com/ovrclk/akash/util/network"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	"github.com/ovrclk/akash/x/escrow/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"

	akashmock "github.com/ovrclk/akash/client/mocks"
	cosmosMock "github.com/ovrclk/akash/testutil/cosmos_mock"
)

type scaffold struct {
	testAddr sdk.AccAddress
	testBus  pubsub.Bus
	ctx      context.Context
	cancel   context.CancelFunc
	qc       *cosmosMock.QueryClient
	aqc      *akashmock.QueryClient
	bc       *balanceChecker
}

func leaseMonitorForTest(t *testing.T) (*scaffold, *balanceChecker) {
	s := &scaffold{}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	myLog := testutil.Logger(t)

	s.testAddr = testutil.AccAddress(t)

	aqc := akashmock.NewQueryClient(t)

	startedAt := time.Now()

	nodeSyncInfo := &tmrpc.SyncInfo{
		LatestBlockHeight: 1,
		CatchingUp:        false,
	}

	client := akashmock.NewClient(t)
	// client.On("Tx").Return(s.broadcast)
	client.On("NodeSyncInfo", mock.Anything).Run(func(args mock.Arguments) {
		nodeSyncInfo.LatestBlockHeight = int64(time.Now().Sub(startedAt) / netutil.AverageBlockTime)
	}).Return(nodeSyncInfo, nil)

	queryClient := &cosmosMock.QueryClient{}

	deploymentResp := &dtypes.QueryDeploymentResponse{
		Deployment: dtypes.Deployment{
			DeploymentID: dtypes.DeploymentID{},
			State:        0,
			Version:      nil,
			CreatedAt:    0,
		},
		EscrowAccount: v1beta2.Account{
			ID:          v1beta2.AccountID{},
			Owner:       "",
			State:       0,
			Balance:     sdk.NewDecCoin("uakt", sdk.NewInt(2000)),
			Transferred: sdk.NewDecCoin("uakt", sdk.NewInt(0)),
			SettledAt:   1,
			Depositor:   "",
			Funds:       sdk.NewDecCoin("uakt", sdk.NewInt(0)),
		},
	}

	aqc.On("Deployment", mock.Anything, mock.Anything).Return(deploymentResp, nil)

	leasesResp := &mtypes.QueryLeasesResponse{
		Leases: []mtypes.QueryLeaseResponse{
			{
				Lease: mtypes.Lease{
					Price: sdk.NewDecCoin("uakt", sdk.NewInt(1500)),
				},
			},
		},
	}
	aqc.On("Leases", mock.Anything, mock.Anything).Return(leasesResp, nil)

	myProvider := &ptypes.Provider{
		Owner:      s.testAddr.String(),
		HostURI:    "http://test.localhost:7443",
		Attributes: nil,
	}
	mySession := session.New(myLog, client, myProvider, -1)
	s.testBus = pubsub.NewBus()

	bc, err := newBalanceChecker(s.ctx, queryClient, aqc, s.testAddr, mySession, s.testBus, BalanceCheckerConfig{
		WithdrawalPeriod:        time.Hour * 24,
		LeaseFundsCheckInterval: time.Second * 10,
	})
	require.NoError(t, err)

	s.qc = queryClient
	s.bc = bc

	return s, bc
}

func TestBalanceCheckerMonitorsFunds(t *testing.T) {
	testScaffold, bc := leaseMonitorForTest(t)
	defer testScaffold.testBus.Close()

	subscriber, err := bc.bus.Subscribe()
	require.NoError(t, err)

	lid := mtypes.LeaseID{
		Owner:    bc.ownAddr.String(),
		DSeq:     1,
		Provider: bc.ownAddr.String(),
	}

	err = testScaffold.testBus.Publish(event.LeaseAddFundsMonitor{LeaseID: lid, IsNewLease: true})
	require.NoError(t, err)

	ev := <-subscriber.Events()
	require.IsType(t, event.LeaseAddFundsMonitor{}, ev)

	select {
	case ev = <-subscriber.Events():
		require.IsType(t, event.LeaseWithdraw{}, ev)
		require.True(t, lid.Equals(ev.(event.LeaseWithdraw).LeaseID))
	case <-time.NewTimer(time.Second * 25).C:
		t.Errorf("has not received lease withdraw message")
	case <-testScaffold.ctx.Done():
		t.Fail()
		return
	}

	testScaffold.qc.AssertExpectations(t)
}

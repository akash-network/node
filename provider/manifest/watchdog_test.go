package manifest

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	broadcastmocks "github.com/ovrclk/akash/client/broadcaster/mocks"
	clientmocks "github.com/ovrclk/akash/client/mocks"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/testutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type watchdogTestScaffold struct {
	client     *clientmocks.Client
	parentCh   chan struct{}
	doneCh     chan dtypes.DeploymentID
	broadcasts chan sdk.Msg
	leaseID    types.LeaseID
	provider   ptypes.Provider
}

func makeWatchdogTestScaffold(t *testing.T, timeout time.Duration) (*watchdog, *watchdogTestScaffold) {
	scaffold := &watchdogTestScaffold{}
	scaffold.parentCh = make(chan struct{})
	scaffold.doneCh = make(chan dtypes.DeploymentID, 1)
	scaffold.broadcasts = make(chan sdk.Msg)
	scaffold.provider = testutil.Provider(t)
	scaffold.leaseID = testutil.LeaseID(t)
	scaffold.leaseID.Provider = scaffold.provider.Owner
	scaffold.broadcasts = make(chan sdk.Msg, 1)

	txClientMock := &broadcastmocks.Client{}
	txClientMock.On("Broadcast", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		scaffold.broadcasts <- args.Get(1).(sdk.Msg)
	}).Return(nil)

	scaffold.client = &clientmocks.Client{}
	scaffold.client.On("Tx").Return(txClientMock)
	sess := session.New(testutil.Logger(t), scaffold.client, &scaffold.provider)
	require.NotNil(t, sess.Client())

	wd := newWatchdog(sess, scaffold.parentCh, scaffold.doneCh, scaffold.leaseID, timeout)

	return wd, scaffold
}

func TestWatchdogTimesout(t *testing.T) {
	wd, scaffold := makeWatchdogTestScaffold(t, 3*time.Second)

	select {
	case <-wd.lc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting on watchdog to stop")
	}

	// Check that close bid was sent
	msg := testutil.ChannelWaitForValue(t, scaffold.broadcasts)
	closeBid, ok := msg.(*types.MsgCloseBid)
	require.True(t, ok)
	require.Equal(t, closeBid.BidID.LeaseID(), scaffold.leaseID)

	deploymentID := testutil.ChannelWaitForValue(t, scaffold.doneCh)
	require.Equal(t, deploymentID, scaffold.leaseID.DeploymentID())
}

func TestWatchdogStops(t *testing.T) {
	wd, scaffold := makeWatchdogTestScaffold(t, 1*time.Minute)

	wd.stop() // ask it to stop immediately

	select {
	case <-wd.lc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting on watchdog to stop")
	}

	// Check that close bid was not sent
	select {
	case <-scaffold.broadcasts:
		t.Fatal("should no have broadcast any message")
	default:
	}

	deploymentID := testutil.ChannelWaitForValue(t, scaffold.doneCh)
	require.Equal(t, deploymentID, scaffold.leaseID.DeploymentID())
}

func TestWatchdogStopsOnParent(t *testing.T) {
	wd, scaffold := makeWatchdogTestScaffold(t, 1*time.Minute)

	close(scaffold.parentCh) // ask it to stop immediately

	select {
	case <-wd.lc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting on watchdog to stop")
	}

	// Check that close bid was not sent
	select {
	case <-scaffold.broadcasts:
		t.Fatal("should no have broadcast any message")
	default:
	}

	deploymentID := testutil.ChannelWaitForValue(t, scaffold.doneCh)
	require.Equal(t, deploymentID, scaffold.leaseID.DeploymentID())
}

package manifest

import (
	"context"
	"github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/util/runner"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	types "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/tendermint/tendermint/libs/log"
	"time"
)

type watchdog struct {
	leaseID types.LeaseID
	timeout time.Duration
	lc      lifecycle.Lifecycle
	sess    session.Session
	ctx     context.Context
	log     log.Logger
}

func newWatchdog(sess session.Session, parent <-chan struct{}, done chan<- dtypes.DeploymentID, leaseID types.LeaseID, timeout time.Duration) *watchdog {
	ctx, cancel := context.WithCancel(context.Background())
	result := &watchdog{
		leaseID: leaseID,
		timeout: timeout,
		lc:      lifecycle.New(),
		sess:    sess,
		ctx:     ctx,
		log:     sess.Log().With("leaseID", leaseID),
	}

	go func() {
		result.lc.WatchChannel(parent)
		cancel()
	}()

	go func() {
		<-result.lc.Done()
		done <- leaseID.DeploymentID()
	}()

	go result.run()

	return result
}

func (wd *watchdog) stop() {
	wd.lc.ShutdownAsync(nil)
}

func (wd *watchdog) run() {
	defer wd.lc.ShutdownCompleted()

	var runch <-chan runner.Result
	var err error

	wd.log.Debug("watchdog start")
	select {
	case <-time.After(wd.timeout):
		// Close the bid, since if this point is reached then a manifest has not been received
		wd.log.Info("watchdog closing bid")

		runch = runner.Do(func() runner.Result {
			return runner.NewResult(nil, wd.sess.Client().Tx().Broadcast(wd.ctx, &types.MsgCloseBid{
				BidID: types.MakeBidID(wd.leaseID.OrderID(), wd.sess.Provider().Address()),
			}))
		})
	case err = <-wd.lc.ShutdownRequest():
	}

	wd.lc.ShutdownInitiated(err)
	if runch != nil {
		result := <-runch
		if err := result.Error(); err != nil {
			wd.log.Error("failed closing bid", "err", err)
		}
	}
}

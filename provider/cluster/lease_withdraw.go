package cluster

import (
	"context"
	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/util/runner"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
	"time"
)

type deploymentWithdrawal struct {
	bus     pubsub.Bus
	session session.Session
	lease   mtypes.LeaseID
	log     log.Logger
	lc      lifecycle.Lifecycle
}

func newDeploymentWithdrawal(dm *deploymentManager) *deploymentWithdrawal {
	m := &deploymentWithdrawal{
		bus:     dm.bus,
		session: dm.session,
		lease:   dm.lease,
		log:     dm.log.With("cmp", "deployment-withdrawal"),
		lc:      lifecycle.New(),
	}

	go m.lc.WatchChannel(dm.lc.ShuttingDown())
	go m.run()

	return m
}

func (dw *deploymentWithdrawal) doWithdrawal(ctx context.Context) error {
	msg := &mtypes.MsgWithdrawLease{
		LeaseID: dw.lease,
	}

	return dw.session.Client().Tx().Broadcast(ctx, msg)
}

func (dw *deploymentWithdrawal) run() {
	defer dw.lc.ShutdownCompleted()
	ctx, cancel := context.WithCancel(context.Background())

	events, err := dw.bus.Subscribe()
	if err != nil {
		dw.log.Error("Could not subscribe to events", "err", err)
	}

	const withdrawalPeriod = 24 * time.Hour
	ticker := time.NewTicker(withdrawalPeriod)

	var result <-chan runner.Result
loop:
	for {
		withdraw := false
		select {

		case err := <-dw.lc.ShutdownRequest():
			dw.log.Debug("shutting down")
			dw.lc.ShutdownInitiated(err)
			break loop

		case <-ticker.C:
			withdraw = true

		case ev := <-events.Events():
			// This event contains no information, so if it is
			// of the correct type attempt a withdrawal
			_, withdraw = ev.(event.LeaseWithdrawNow)
		case r := <-result:
			result = nil
			if err := r.Error(); err != nil {
				dw.log.Error("failed to do withdrawal", "err", err)
			}
			ticker.Reset(withdrawalPeriod)
		}

		if withdraw {
			ticker.Stop()
			// do the withdrawal
			result = runner.Do(func() runner.Result {
				return runner.NewResult(nil, dw.doWithdrawal(ctx))
			})
		}

	}
	cancel()

	dw.log.Debug("shutdown complete")
}

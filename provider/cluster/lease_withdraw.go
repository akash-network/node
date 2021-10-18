package cluster

import (
	"context"
	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	"github.com/ovrclk/akash/util/runner"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/rand"
	"time"
)

type deploymentWithdrawal struct {
	bus           pubsub.Bus
	session       session.Session
	lease         mtypes.LeaseID
	log           log.Logger
	lc            lifecycle.Lifecycle
	withdrawDelay time.Duration
}

var (
	leaseWithdrawalCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_lease_withdrawal",
	}, []string{"result"})
)

const baseWithdrawDelay = time.Second * 12
const withdrawDelayModulus = 50

func newDeploymentWithdrawal(dm *deploymentManager, delayIndex uint, baseDelay time.Duration) *deploymentWithdrawal {
	// Compute a delay based off the index passed in
	withdrawDelay := time.Duration(0)
	if delayIndex > 0 {
		withdrawDelay = baseWithdrawDelay
		delayIndex %= withdrawDelayModulus // Prevent this value from becoming very large
		withdrawDelay *= time.Duration(delayIndex)

		randOffset := rand.Float64() - 0.5
		randOffset *= float64(baseWithdrawDelay.Milliseconds())
		withdrawDelay += time.Millisecond * time.Duration(randOffset)
	}
	// add in the base time
	withdrawDelay += baseDelay

	m := &deploymentWithdrawal{
		bus:           dm.bus,
		session:       dm.session,
		lease:         dm.lease,
		log:           dm.log.With("cmp", "deployment-withdrawal"),
		lc:            lifecycle.New(),
		withdrawDelay: withdrawDelay,
	}

	go m.lc.WatchChannel(dm.lc.ShuttingDown())
	go m.run()

	return m
}

func (dw *deploymentWithdrawal) doWithdrawal(ctx context.Context) error {
	if dw.withdrawDelay > time.Duration(0) {
		after := time.After(dw.withdrawDelay)

		// Wait until the context is cancelled, or the delay elapses
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-after:
		}
	}

	msg := &mtypes.MsgWithdrawLease{
		LeaseID: dw.lease,
	}

	result := dw.session.Client().Tx().Broadcast(ctx, msg)

	label := metricsutils.SuccessLabel
	if result != nil {
		label = metricsutils.FailLabel
	}
	leaseWithdrawalCounter.WithLabelValues(label).Inc()
	return result
}

func (dw *deploymentWithdrawal) run() {
	defer dw.lc.ShutdownCompleted()
	ctx, cancel := context.WithCancel(context.Background())

	events, err := dw.bus.Subscribe()
	if err != nil {
		dw.log.Error("Could not subscribe to events", "err", err)
	}
	defer events.Close()

	var result <-chan runner.Result

loop:
	for {
		withdraw := false
		select {

		case err := <-dw.lc.ShutdownRequest():
			dw.log.Debug("shutting down")
			dw.lc.ShutdownInitiated(err)
			break loop
		case ev := <-events.Events():
			// This event contains no information, so if it is
			// of the correct type attempt a withdrawal
			_, withdraw = ev.(event.LeaseWithdrawNow)
		case r := <-result:
			result = nil
			if err := r.Error(); err != nil {
				dw.log.Error("failed to do withdrawal", "err", err)
			}
		}

		if withdraw {
			// do the withdrawal
			result = runner.Do(func() runner.Result {
				return runner.NewResult(nil, dw.doWithdrawal(ctx))
			})
		}

	}
	cancel()

	dw.log.Debug("shutdown complete")
}

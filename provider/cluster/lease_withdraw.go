package cluster

import (
	"context"

	"github.com/boz/go-lifecycle"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	"github.com/ovrclk/akash/util/runner"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type deploymentWithdrawal struct {
	session session.Session
	lease   mtypes.LeaseID
	log     log.Logger
	lc      lifecycle.Lifecycle
}

var (
	leaseWithdrawalCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_lease_withdrawal",
	}, []string{"result"})
)

func newDeploymentWithdrawal(dm *deploymentManager) (*deploymentWithdrawal, error) {
	m := &deploymentWithdrawal{
		session: dm.session,
		lease:   dm.lease,
		log:     dm.log.With("cmp", "deployment-withdrawal"),
		lc:      lifecycle.New(),
	}

	events, err := dm.bus.Subscribe()
	if err != nil {
		return nil, err
	}

	go m.lc.WatchChannel(dm.lc.ShuttingDown())
	go m.run(events)

	return m, nil
}

func (dw *deploymentWithdrawal) doWithdrawal(ctx context.Context) error {
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

func (dw *deploymentWithdrawal) run(events pubsub.Subscriber) {
	defer func() {
		dw.lc.ShutdownCompleted()
		events.Close()
	}()
	ctx, cancel := context.WithCancel(context.Background())

	var result <-chan runner.Result
loop:
	for {
		select {
		case err := <-dw.lc.ShutdownRequest():
			dw.log.Debug("shutting down")
			dw.lc.ShutdownInitiated(err)
			break loop
		case ev := <-events.Events():
			if evt, valid := ev.(event.LeaseWithdraw); valid {
				if !evt.LeaseID.Equals(dw.lease) {
					continue loop
				}

				// do the withdrawal
				result = runner.Do(func() runner.Result {
					return runner.NewResult(nil, dw.doWithdrawal(ctx))
				})
			}
		case r := <-result:
			result = nil
			if err := r.Error(); err != nil {
				dw.log.Error("failed to do withdrawal", "err", err)
			}
		}
	}
	cancel()

	if result != nil {
		// The context has been cancelled, so wait for the result now and discard it
		<-result
	}

	dw.log.Debug("shutdown complete")
}

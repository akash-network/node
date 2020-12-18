package cluster

import (
	"context"
	"math/rand"
	"time"

	lifecycle "github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/util/runner"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	monitorMaxRetries        = 40
	monitorRetryPeriodMin    = time.Second * 4
	monitorRetryPeriodJitter = time.Second * 15

	monitorHealthcheckPeriodMin    = time.Second * 10
	monitorHealthcheckPeriodJitter = time.Second * 5
)

type deploymentMonitor struct {
	bus     pubsub.Bus
	session session.Session
	client  Client

	lease  mtypes.LeaseID
	mgroup *manifest.Group

	attempts int
	log      log.Logger
	lc       lifecycle.Lifecycle
}

func newDeploymentMonitor(dm *deploymentManager) *deploymentMonitor {
	m := &deploymentMonitor{
		bus:     dm.bus,
		session: dm.session,
		client:  dm.client,
		lease:   dm.lease,
		mgroup:  dm.mgroup,
		log:     dm.log.With("cmp", "deployment-monitor"),
		lc:      lifecycle.New(),
	}

	go m.lc.WatchChannel(dm.lc.ShuttingDown())
	go m.run()

	return m
}

func (m *deploymentMonitor) shutdown() {
	m.lc.ShutdownAsync(nil)
}

func (m *deploymentMonitor) done() <-chan struct{} {
	return m.lc.Done()
}

func (m *deploymentMonitor) run() {
	defer m.lc.ShutdownCompleted()

	var (
		runch   <-chan runner.Result
		closech <-chan runner.Result
	)

	tickch := m.scheduleRetry()

loop:
	for {
		select {

		case err := <-m.lc.ShutdownRequest():
			m.lc.ShutdownInitiated(err)
			break loop

		case <-tickch:
			tickch = nil
			runch = m.runCheck()

		case result := <-runch:
			runch = nil

			if err := result.Error(); err != nil {
				m.log.Error("monitor check", "err", err)
			}

			ok := result.Value().(bool)

			m.log.Info("check result", "ok", ok, "attempt", m.attempts)

			if ok {
				// healthy
				m.attempts = 0
				tickch = m.scheduleHealthcheck()
				m.publishStatus(event.ClusterDeploymentDeployed)
				break
			}

			m.publishStatus(event.ClusterDeploymentPending)

			if m.attempts <= monitorMaxRetries {
				// unhealthy.  retry
				tickch = m.scheduleRetry()
				break
			}

			m.log.Error("deployment failed.  closing lease.")
			closech = m.runCloseLease()

		case <-closech:
			closech = nil
		}
	}

	if runch != nil {
		<-runch
	}

	if closech != nil {
		<-closech
	}
}

func (m *deploymentMonitor) runCheck() <-chan runner.Result {
	m.attempts++
	m.log.Debug("running check", "attempt", m.attempts)
	return runner.Do(func() runner.Result {
		return runner.NewResult(m.doCheck())
	})
}

func (m *deploymentMonitor) doCheck() (bool, error) {
	ctx := context.Background() // TODO: manage context within the deploymentMonitor{}
	status, err := m.client.LeaseStatus(ctx, m.lease)

	if err != nil {
		m.log.Error("lease status", "err", err)
		return false, err
	}

	badsvc := 0

	for _, spec := range m.mgroup.Services {
		service, foundService := status.Services[spec.Name]
		if foundService {
			if uint32(service.Available) < spec.Count {
				badsvc++
				m.log.Debug("service available replicas below target",
					"service", spec.Name,
					"available", service.Available,
					"target", spec.Count,
				)
			}
		}

		if !foundService {
			badsvc++
			m.log.Debug("service status not found", "service", spec.Name)
		}
	}

	return badsvc == 0, nil
}

func (m *deploymentMonitor) runCloseLease() <-chan runner.Result {
	return runner.Do(func() runner.Result {
		// TODO: retry, timeout
		err := m.session.Client().Tx().Broadcast(context.Background(), &mtypes.MsgCloseBid{
			BidID: m.lease.BidID(),
		})
		if err != nil {
			m.log.Error("closing deployment", "err", err)
		} else {
			m.log.Info("bidding on lease closed")
		}
		return runner.NewResult(nil, err)
	})
}

func (m *deploymentMonitor) publishStatus(status event.ClusterDeploymentStatus) {
	if err := m.bus.Publish(event.ClusterDeployment{
		LeaseID: m.lease,
		Group:   m.mgroup,
		Status:  status,
	}); err != nil {
		m.log.Error("publishing manifest group deployed event", "err", err, "status", status)
	}
}

func (m *deploymentMonitor) scheduleRetry() <-chan time.Time {
	return m.schedule(monitorRetryPeriodMin, monitorRetryPeriodJitter)
}

func (m *deploymentMonitor) scheduleHealthcheck() <-chan time.Time {
	return m.schedule(monitorHealthcheckPeriodMin, monitorHealthcheckPeriodJitter)
}

func (m *deploymentMonitor) schedule(min, jitter time.Duration) <-chan time.Time {
	period := min + time.Duration(rand.Int63n(int64(jitter))) // nolint: gosec
	return time.After(period)
}

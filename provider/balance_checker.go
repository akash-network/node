package provider

import (
	"context"
	"github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	putil "github.com/ovrclk/akash/provider/util"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/util/runner"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tendermint/tendermint/libs/log"

	"time"
)

var (
	balanceGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "provider_balance",
	})
)

type balanceChecker struct {
	session         session.Session
	log             log.Logger
	lc              lifecycle.Lifecycle
	bus             pubsub.Bus
	ownAddr         sdk.AccAddress
	bankQueryClient bankTypes.QueryClient

	cfg BalanceCheckerConfig

	balanceCheckDelay time.Duration
}

type BalanceCheckerConfig struct {
	PollingPeriod           time.Duration
	MinimumBalanceThreshold uint64
	WithdrawalPeriod        time.Duration
}

func newBalanceChecker(ctx context.Context,
	bankQueryClient bankTypes.QueryClient,
	accAddr sdk.AccAddress,
	clientSession session.Session,
	bus pubsub.Bus,
	cfg BalanceCheckerConfig) *balanceChecker {

	balanceCheckDelay := putil.PseudoRandomUintFromAddr(clientSession.Provider().GetOwner(), 10000)

	bc := &balanceChecker{
		session: clientSession,
		log:     clientSession.Log().With("cmp", "balance-checker"),
		lc:      lifecycle.New(),
		bus:     bus,
		ownAddr: accAddr,

		bankQueryClient:   bankQueryClient,
		cfg:               cfg,
		balanceCheckDelay: time.Duration(balanceCheckDelay) * time.Millisecond,
	}

	go bc.lc.WatchContext(ctx)
	go bc.run()

	return bc
}
func (bc *balanceChecker) doCheck(ctx context.Context) (bool, error) {
	// if a bunch of providrs are restarted at the same time they could
	// stack up and hit the same RPC node. Space this out since it isn't time critical

	// Get the current wallet balance
	query := bankTypes.NewQueryBalanceRequest(bc.ownAddr, "uakt")
	result, err := bc.bankQueryClient.Balance(ctx, query)
	if err != nil {
		return false, err
	}

	balance := result.Balance.Amount
	balanceGauge.Set(float64(balance.Uint64()))

	if bc.cfg.MinimumBalanceThreshold == 0 {
		return false, nil
	}

	tooLow := sdk.NewIntFromUint64(bc.cfg.MinimumBalanceThreshold).GT(balance)

	return tooLow, nil
}

func (bc *balanceChecker) startWithdrawAll() error {
	return bc.bus.Publish(event.LeaseWithdrawNow{})
}

func (bc *balanceChecker) run() {
	defer bc.lc.ShutdownCompleted()
	ctx, cancel := context.WithCancel(context.Background())

	tick := time.NewTicker(bc.cfg.PollingPeriod)
	withdrawalTicker := time.NewTicker(bc.cfg.WithdrawalPeriod)

	var balanceCheckResult <-chan runner.Result
	var withdrawAllResult <-chan runner.Result

loop:
	for {
		withdrawAllNow := false

		select {

		case err := <-bc.lc.ShutdownRequest():
			bc.log.Debug("shutting down")
			bc.lc.ShutdownInitiated(err)
			break loop
		case <-tick.C:
			tick.Stop() // Stop the timer
			// Start the balance check
			balanceCheckResult = runner.Do(func() runner.Result {
				return runner.NewResult(bc.doCheck(ctx))
			})

		case balanceCheck := <-balanceCheckResult:
			balanceCheckResult = nil
			tick.Reset(bc.cfg.PollingPeriod) // Re-enable the timer
			err := balanceCheck.Error()
			if err != nil {
				bc.log.Error("failed to check balance", "err", err)
				break
			}

			tooLow := balanceCheck.Value().(bool)
			if tooLow {
				// trigger the withdrawal
				bc.log.Info("balance below target amount")
				withdrawAllNow = true
			}
		case withdrawAll := <-withdrawAllResult:

			withdrawAllResult = nil
			withdrawalTicker.Reset(bc.cfg.PollingPeriod) // Re-enable the timer
			if err := withdrawAll.Error(); err != nil {
				bc.log.Error("failed to started withdrawals", "err", err)
			}
		case <-withdrawalTicker.C:
			withdrawAllNow = true
			withdrawalTicker.Stop()
		}

		if withdrawAllNow {
			bc.log.Info("balance below target amount, withdrawing now")
			withdrawAllResult = runner.Do(func() runner.Result {
				return runner.NewResult(nil, bc.startWithdrawAll())
			})
		}
	}
	cancel()

	bc.log.Debug("shutdown complete")
}

package provider

import (
	"context"
	"github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/util/runner"
	mparams "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"

	"time"
)

type balanceChecker struct {
	session         session.Session
	log             log.Logger
	lc              lifecycle.Lifecycle
	bus             pubsub.Bus
	ownAddr         sdk.AccAddress
	checkPeriod     time.Duration
	bankQueryClient bankTypes.QueryClient
}

func newBalanceChecker(ctx context.Context, bankQueryClient bankTypes.QueryClient, accAddr sdk.AccAddress, clientSession session.Session, bus pubsub.Bus) *balanceChecker {
	bc := &balanceChecker{

		session:         clientSession,
		log:             clientSession.Log().With("cmp", "balance-checker"),
		lc:              lifecycle.New(),
		bus:             bus,
		ownAddr:         accAddr,
		checkPeriod:     5 * time.Minute,
		bankQueryClient: bankQueryClient,
	}

	go bc.lc.WatchContext(ctx)
	go bc.run()

	return bc
}
func (bc *balanceChecker) doCheck(ctx context.Context) (bool, error) {
	// Get the current wallet balance
	query := bankTypes.NewQueryBalanceRequest(bc.ownAddr, "uakt")
	result, err := bc.bankQueryClient.Balance(ctx, query)
	if err != nil {
		return false, err
	}

	balance := result.Balance.Amount
	bc.log.Debug("provider acct balance", "balance", balance)
	// Get the amount required as a bid deposit
	// TODO - pull me from the blockchain in the future
	defaultMinBidDeposit := mparams.DefaultBidMinDeposit

	// Check to see if 2x the minimum bid deposit is greater than wallet balance
	tooLow := defaultMinBidDeposit.Amount.Mul(sdk.NewInt(2)).GT(balance)
	return tooLow, nil
}

func (bc *balanceChecker) startWithdrawAll() error {
	return bc.bus.Publish(event.LeaseWithdrawNow{})
}

func (bc *balanceChecker) run() {
	defer bc.lc.ShutdownCompleted()
	ctx, cancel := context.WithCancel(context.Background())

	tick := time.NewTicker(bc.checkPeriod)
	var balanceCheckResult <-chan runner.Result
	var withdrawAllResult <-chan runner.Result
loop:
	for {
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
			err := balanceCheck.Error()
			if err != nil {
				bc.log.Error("failed to check balance", "err", err)
				tick.Reset(bc.checkPeriod) // Re-enable the timer
				break
			}

			tooLow := balanceCheck.Value().(bool)
			if tooLow {
				// trigger the withdrawal
				bc.log.Info("balance below target amount, withdrawing now")
				withdrawAllResult = runner.Do(func() runner.Result {
					return runner.NewResult(nil, bc.startWithdrawAll())
				})
			}
		case withdrawAll := <-withdrawAllResult:
			withdrawAllResult = nil
			if err := withdrawAll.Error(); err != nil {
				bc.log.Error("failed to started withdrawals", "err", err)
			}
			tick.Reset(bc.checkPeriod) // Re-enable the timer
		}
	}
	cancel()

	bc.log.Debug("shutdown complete")
}

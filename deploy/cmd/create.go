package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ovrclk/akash/cmd/common"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

	"github.com/avast/retry-go"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	gateway "github.com/ovrclk/akash/provider/gateway/rest"
	dcli "github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

const (
	// FlagTimeout represents max amount of time for lease status checking process
	FlagTimeout = "timeout"
	// FlagTick represents time interval at which lease status is checked
	FlagTick = "tick"
)

func retryIfGatewayClientResponseError(err error) bool {
	_, isClientErr := err.(gateway.ClientResponseError)
	return isClientErr
}

var errDeployTimeout = errors.New("Timed out while trying to deploy")
var DefaultDeposit = dtypes.DefaultDeploymentMinDeposit

// createCmd represents the create command
func createCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create [sdl-file]",
		Args:  cobra.ExactArgs(1),
		Short: "Create a deployment on the akash network",
		RunE: func(cmd *cobra.Command, args []string) error {
			timeoutDuration := viper.GetDuration(FlagTimeout)
			endAt := time.Now().Add(timeoutDuration)
			ctx, cancel := context.WithDeadline(cmd.Context(), endAt)
			tickDuration := viper.GetDuration(FlagTick)

			maxDelay := tickDuration
			const defaultMaxDelay = 15 * time.Second
			if maxDelay < defaultMaxDelay {
				maxDelay = defaultMaxDelay
			}

			retryConfiguration := []retry.Option{
				retry.DelayType(retry.BackOffDelay),
				retry.Attempts(9999), // Use a large number here, since a deadline is used on the context
				retry.MaxDelay(maxDelay),
				retry.Delay(tickDuration),
				retry.RetryIf(retryIfGatewayClientResponseError),
				retry.Context(ctx),
			}

			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			gClientDir, err := gateway.NewClientDirectory(cmd.Context(), clientCtx)
			if err != nil {
				if os.IsNotExist(err) {
					err = errors.Errorf("no certificate file found for account %q.\n"+
						"consider creating it as certificate required to create a deployment", clientCtx.FromAddress.String())
				}
				return err
			}

			log := logger.With("cli", "create")
			dd, err := NewDeploymentData(args[0], cmd.Flags(), clientCtx)
			if err != nil {
				return err
			}

			group, _ := errgroup.WithContext(ctx)

			// Listen to on chain events and send the manifest when required
			leasesReady := make(chan struct{}, 1)
			bids := make(chan mtypes.EventBidCreated, 1)
			group.Go(func() error {
				if err = ChainEmitter(ctx, clientCtx, DeploymentDataUpdateHandler(dd, bids, leasesReady), SendManifestHander(clientCtx, dd, gClientDir, retryConfiguration)); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error watching events", "err", err)
					cancel()
				}
				return err
			})

			// Send the deployment creation transaction
			group.Go(func() error {
				if err = TxCreateDeployment(clientCtx, cmd.Flags(), dd); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error creating deployment", "err", err)
					cancel()
				}
				return err
			})

			wfb := newWaitForBids(dd, bids)
			group.Go(func() error {
				if err = wfb.run(ctx, cancel, clientCtx, cmd.Flags()); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error waiting for bids to be made", "err", err)
					cancel()
				}
				return err
			})

			wfl := newWaitForLeases(dd, gClientDir, retryConfiguration, leasesReady)
			// Wait for the leases to be created and then start polling the provider for service availability
			group.Go(func() error {
				if err = wfl.run(ctx, cancel); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error waiting for services to be ready", "err", err)
					cancel()
				}
				return err
			})

			// This returns "context cancelled" when everything goes OK
			err = group.Wait()
			cancel()
			if err != nil && errors.Is(err, context.Canceled) && wfl.allLeasesOk {
				err = nil // Not an actual error to stop on
			}

			if err != nil {
				return err
			}

			// Reset the context
			ctx, cancel = context.WithDeadline(cmd.Context(), endAt)
			err = wfl.eachService(func(leaseID mtypes.LeaseID, serviceName string) error {
				gclient, err := gClientDir.GetClientFromBech32(leaseID.Provider)
				if err != nil {
					return err
				}

				var status *ctypes.ServiceStatus
				if err = retry.Do(func() error {
					status, err = gclient.ServiceStatus(ctx, leaseID, serviceName)
					return err
				}); err != nil {
					return err
				}

				// Encode and show the response
				statusEncoded, err := json.MarshalIndent(status, "", " ")
				if err != nil {
					return nil
				}

				_, err = os.Stdout.Write(statusEncoded)
				if err != nil {
					return err
				}
				_, err = fmt.Print("\n")
				return err
			})
			cancel()

			if errors.Is(err, context.Canceled) {
				return errDeployTimeout
			}

			return err
		},
	}

	cmd.Flags().String(flags.FlagChainID, "", "The network chain ID")
	if err := viper.BindPFlag(flags.FlagChainID, cmd.Flags().Lookup(flags.FlagChainID)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagTimeout, 150*time.Second, "The max amount of time to wait for deployment status checking process")
	if err := viper.BindPFlag(FlagTimeout, cmd.Flags().Lookup(FlagTimeout)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagTick, 500*time.Millisecond, "The time interval at which deployment status is checked")
	if err := viper.BindPFlag(FlagTick, cmd.Flags().Lookup(FlagTick)); err != nil {
		return nil
	}

	flags.AddTxFlagsToCmd(cmd)
	dcli.AddDeploymentIDFlags(cmd.Flags())
	common.AddDepositFlags(cmd.Flags(), DefaultDeposit)

	return cmd
}

func newWaitForBids(dd *DeploymentData, bids <-chan mtypes.EventBidCreated) *waitForBids {
	return &waitForBids{
		bids:       bids,
		groupCount: len(dd.Groups),
	}
}

type waitForBids struct {
	groupCount int
	bids       <-chan mtypes.EventBidCreated
}

type bidList []mtypes.EventBidCreated

func (bl bidList) Len() int {
	return len(bl)
}

func (bl bidList) Less(i, j int) bool {
	lhs := bl[i]
	rhs := bl[j]

	if !lhs.Price.Amount.Equal(rhs.Price.Amount) {
		return lhs.Price.Amount.LT(rhs.Price.Amount)
	}

	return lhs.ID.Provider < rhs.ID.Provider
}

func (bl bidList) Swap(i, j int) {
	bl[i], bl[j] = bl[j], bl[i]
}

func (wfb *waitForBids) run(ctx context.Context, cancel context.CancelFunc, clientCtx client.Context, flags *pflag.FlagSet) error {
	var bidDeadline <-chan time.Time
	allBids := make(map[uint32]bidList)
loop:
	for {
		select {
		case bid := <-wfb.bids:
			logger.Debug("Processing bid")
			bidsForGroup := allBids[bid.ID.GSeq]

			allBids[bid.ID.GSeq] = append(bidsForGroup, bid)

			// If there is a bid for at least every group, then start the deadline
			if nil == bidDeadline && len(allBids) == wfb.groupCount {
				logger.Debug("All groups have at least one bid")
				// TODO - this value was made up
				ticker := time.NewTicker(time.Second * 15)
				bidDeadline = ticker.C
				defer ticker.Stop()
			}

		case <-ctx.Done():
			cancel()
			return context.Canceled
		case <-bidDeadline:
			logger.Info("Done waiting on bids", "qty", len(allBids))
			break loop
		}
	}

	for gseq, bidsForGroup := range allBids {
		// Create the lease, using the lowest price
		sort.Sort(bidsForGroup)
		winningBid := bidsForGroup[0]

		// check for more than 1 bid having the same price
		if len(allBids) > 1 && winningBid.Price.Equal(bidsForGroup[1].Price) {
			identical := make(bidList, 0)
			for _, bid := range bidsForGroup {
				if bid.Price.Equal(winningBid.Price) {
					identical = append(identical, bid)
				}
			}
			logger.Info("Multiple bids with identical price", "gseq", gseq, "price", winningBid.Price.String(), "qty", len(identical))
			rng := rand.New(rand.NewSource(int64(winningBid.ID.DSeq))) //nolint
			choice := rng.Intn(len(identical))

			winningBid = identical[choice]
		}

		logger.Info("Winning bid", "gseq", gseq, "price", winningBid.Price.String(), "provider", winningBid.ID.Provider)

		ev := &mtypes.MsgCreateLease{
			BidID: winningBid.ID,
		}

		_, err := SendMsgs(clientCtx, flags, []sdk.Msg{ev})
		if err != nil {
			return err
		}

	}

	return nil
}

func newWaitForLeases(dd *DeploymentData, gClientDir *gateway.ClientDirectory, retryConfiguration []retry.Option, leasesReady <-chan struct{}) *waitForLeases {
	return &waitForLeases{
		dd:                 dd,
		gClientDir:         gClientDir,
		leasesReady:        leasesReady,
		retryConfiguration: retryConfiguration,
		allLeasesOk:        false,
	}
}

type leaseAndService struct {
	leaseID     mtypes.LeaseID
	serviceName string
}

type waitForLeases struct {
	dd                 *DeploymentData
	gClientDir         *gateway.ClientDirectory
	leasesReady        <-chan struct{}
	retryConfiguration []retry.Option
	allLeasesOk        bool
	services           []leaseAndService
	lock               sync.Mutex
}

func (wfl *waitForLeases) eachService(fn func(leaseID mtypes.LeaseID, serviceName string) error) error {
	for _, entry := range wfl.services {
		err := fn(entry.leaseID, entry.serviceName)
		if err != nil {
			return err
		}
	}
	return nil
}

var errLeaseNotReady = errors.New("lease not ready")

// WaitForLeasesAndPollService waits for leases
func (wfl *waitForLeases) run(ctx context.Context, cancel context.CancelFunc) error {
	log := logger

	// Wait for signal that expected leases exist
	select {
	case <-wfl.leasesReady:

	case <-ctx.Done():
		cancel()
		return context.Canceled
	}

	leases := wfl.dd.Leases()
	log.Info("Waiting on leases to be ready", "leaseQuantity", len(leases))

	var localRetryConfiguration []retry.Option
	localRetryConfiguration = append(localRetryConfiguration, wfl.retryConfiguration...)

	retryIf := func(err error) bool {
		if retryIfGatewayClientResponseError(err) {
			return true
		}

		return errors.Is(err, errLeaseNotReady)
	}
	localRetryConfiguration = append(localRetryConfiguration, retry.RetryIf(retryIf))

	leaseChecker := func(leaseID mtypes.LeaseID) (func() error, error) {
		log.Debug("Checking status of lease", "lease", leaseID)

		gclient, err := wfl.gClientDir.GetClientFromBech32(leaseID.GetProvider())
		if err != nil {
			cancel()
			return nil, err
		}

		servicesChecked := make(map[string]bool)

		return func() error {
			err = retry.Do(func() error {
				ls, err := gclient.LeaseStatus(ctx, leaseID)

				if err != nil {
					log.Debug("Could not get lease status", "lease", leaseID, "err", err)
					return err
				}

				for serviceName, s := range ls.Services {
					checked := servicesChecked[serviceName]
					if checked {
						continue
					}
					isOk := s.Available == s.Total
					if !isOk {
						return fmt.Errorf("%w: service %q has %d / %d available", errLeaseNotReady, serviceName, s.Available, s.Total)
					}
					servicesChecked[serviceName] = true
					log.Info("service ready", "lease", leaseID, "service", serviceName)
				}

				// Update the shared data
				wfl.lock.Lock()
				defer wfl.lock.Unlock()
				for serviceName := range ls.Services {
					wfl.services = append(wfl.services, leaseAndService{
						leaseID:     leaseID,
						serviceName: serviceName,
					})
				}
				return nil
			}, localRetryConfiguration...)
			if err != nil {
				return err
			}

			log.Info("lease ready", "leaseID", leaseID)
			return nil
		}, nil
	}

	group, _ := errgroup.WithContext(ctx)

	for _, leaseID := range leases {
		fn, err := leaseChecker(leaseID)
		if err != nil {
			return err
		}
		group.Go(fn)
	}

	err := group.Wait()
	if err == nil { // If all return without error, then all leases are ready
		wfl.allLeasesOk = true
	}
	cancel()
	return nil
}

// TxCreateDeployment takes DeploymentData and creates the specified deployment
func TxCreateDeployment(clientCtx client.Context, flags *pflag.FlagSet, dd *DeploymentData) (err error) {
	res, err := SendMsgs(clientCtx, flags, []sdk.Msg{dd.MsgCreate()})
	log := logger.With(
		"msg", "create-deployment",
	)

	if err != nil || res == nil || res.Code != 0 {
		log.Error("tx failed")
		return err
	}

	log = logger.With(
		"hash", res.TxHash,
		"code", res.Code,
		"codespace", res.Codespace,
		"action", "create-deployment",
		"dseq", dd.DeploymentID.DSeq,
	)

	log.Info("tx sent successfully")
	return nil
}

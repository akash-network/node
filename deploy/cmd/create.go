package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"os"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/gateway"
	dcli "github.com/ovrclk/akash/x/deployment/client/cli"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
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

var (
	errTimeout = errors.New("timed out listening for deployment to be available")
)

// createCmd represents the create command
func createCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [sdl-file]",
		Args:  cobra.ExactArgs(1),
		Short: "Create a deployment to be managed by the deploy application",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			log := logger.With("cli", "create")
			dd, err := NewDeploymentData(args[0], cmd.Flags(), clientCtx)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			group, _ := errgroup.WithContext(ctx)

			// Listen to on chain events and send the manifest when required
			group.Go(func() error {
				if err = ChainEmitter(ctx, clientCtx, DeploymentDataUpdateHandler(dd), SendManifestHander(clientCtx, dd)); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error watching events", "err", err)
				}
				return err
			})

			// Send the deployment creation transaction
			group.Go(func() error {
				if err = TxCreateDeployment(clientCtx, cmd.Flags(), dd); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error creating deployment", "err", err)
				}
				return err
			})

			wfl := newWaitForLeases(dd)
			// Wait for the leases to be created and then start polling the provider for service availability
			group.Go(func() error {
				if err = wfl.run(clientCtx, cancel); err != nil && !errors.Is(err, context.Canceled) {
					log.Error("error waiting for services to be ready", "err", err)
				}
				return err
			})

			// This returns "context cancelled" when everything goes OK
			err = group.Wait()
			if err != nil && errors.Is(err, context.Canceled) && wfl.allLeasesOk() {
				err = nil // Not an actual error to stop on
			}

			if err != nil {
				return err
			}

			gclient := gateway.NewClient()

			deadline := time.After(viper.GetDuration(FlagTimeout))
			ctx, cancel = context.WithCancel(context.Background())
			go func() {
				<-deadline
				cancel()
			}()

			return wfl.eachService(func(leaseID mtypes.LeaseID, providerHost string, serviceName string) error {
				var status *ctypes.ServiceStatus
				var err error
			loop:
				for {
					status, err = gclient.ServiceStatus(ctx, providerHost, leaseID, serviceName)
					if err != nil {
						_, isClientErr := err.(gateway.ClientResponseError)
						if isClientErr {
							time.Sleep(time.Second * 3) // Delay before next attempt
							continue
						}

						return err
					}
					// Got status, so terminate the loop
					break loop
				}

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
		},
	}

	cmd.Flags().String(flags.FlagChainID, "", "The network chain ID")
	if err := viper.BindPFlag(flags.FlagChainID, cmd.Flags().Lookup(flags.FlagChainID)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagTimeout, 150*time.Second, "The max amount of time for lease status checking process")
	if err := viper.BindPFlag(FlagTimeout, cmd.Flags().Lookup(FlagTimeout)); err != nil {
		return nil
	}

	cmd.Flags().Duration(FlagTick, 500*time.Millisecond, "The time interval at which lease status is checked")
	if err := viper.BindPFlag(FlagTick, cmd.Flags().Lookup(FlagTick)); err != nil {
		return nil
	}

	flags.AddTxFlagsToCmd(cmd)
	dcli.AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func getProviderHostURIFromLease(clientCtx client.Context, provider string) (string, error) {
	pclient := pmodule.AppModuleBasic{}.GetQueryClient(clientCtx)

	// Retrieve the provider host URI
	var p *ptypes.Provider
	if err := retry.Do(func() error {
		res, err := pclient.Provider(
			context.Background(),
			&ptypes.QueryProviderRequest{Owner: provider},
		)
		if err != nil {
			// TODO: Log retry?
			return err
		}

		p = &res.Provider
		return nil
	}); err != nil {
		return "", fmt.Errorf("error querying provider: %w", err)
	}

	return p.HostURI, nil
}

func newWaitForLeases(dd *DeploymentData) *waitForLeases {
	return &waitForLeases{
		dd:                 dd,
		servicesOkForLease: make(map[mtypes.LeaseID]map[string]bool),
		providers:          make(map[string]string),
	}
}

type waitForLeases struct {
	dd                 *DeploymentData
	servicesOkForLease map[mtypes.LeaseID]map[string]bool
	providers          map[string]string
}

func (wfl *waitForLeases) leaseIsOk(leaseID mtypes.LeaseID) bool {
	data := wfl.servicesOkForLease[leaseID]
	if len(data) == 0 {
		return false // No data stored, lease is not OK
	}

	for _, isOk := range data {
		if !isOk {
			return false
		}
	}

	return true
}

func (wfl *waitForLeases) allLeasesOk() bool {
	if len(wfl.servicesOkForLease) == 0 {
		return false
	}
	for _, leaseID := range wfl.dd.Leases() {
		if !wfl.leaseIsOk(leaseID) {
			return false
		}
	}

	return true
}

func (wfl *waitForLeases) eachService(fn func(leaseID mtypes.LeaseID, providerHost string, serviceName string) error) error {
	for leaseID, services := range wfl.servicesOkForLease {
		providerHost := wfl.providers[leaseID.Provider]

		for serviceName := range services {
			err := fn(leaseID, providerHost, serviceName)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// WaitForLeasesAndPollService waits for leases
func (wfl *waitForLeases) run(clientCtx client.Context, cancel context.CancelFunc) error {
	log := logger

	timeoutDuration := viper.GetDuration(FlagTimeout)
	tickDuration := viper.GetDuration(FlagTick)
	timeout := time.After(timeoutDuration)
	tick := time.Tick(tickDuration)

	for {
		select {
		case <-timeout:
			log.Error(errTimeout.Error())
			cancel()
			return errTimeout
		case <-tick:
			if wfl.dd.ExpectedLeases() {
				for _, leaseID := range wfl.dd.Leases() {
					if wfl.leaseIsOk(leaseID) {
						continue
					}
					var err error
					// Lookup provider host URI
					providerHostURI, ok := wfl.providers[leaseID.Provider]
					if !ok { // Fetch it if needed
						providerHostURI, err = getProviderHostURIFromLease(clientCtx, leaseID.Provider)
						if err != nil {
							cancel()
							return fmt.Errorf("error getting provider URI: %w", err)
						}
						wfl.providers[leaseID.Provider] = providerHostURI // Fill in the data in the map for next time
					}

					var ls *ctypes.LeaseStatus
					if err := retry.Do(func() error {
						ls, err = gateway.NewClient().LeaseStatus(context.Background(), providerHostURI, leaseID)
						if err != nil {
							return err
						}
						return nil
					}); err != nil {
						cancel()
						return fmt.Errorf("error querying lease status: %w", err)
					}

					servicesStatus, exists := wfl.servicesOkForLease[leaseID]
					if !exists {
						servicesStatus = make(map[string]bool)
						wfl.servicesOkForLease[leaseID] = servicesStatus
					}
					for serviceName, s := range ls.Services {
						isOk := s.Available == s.Total
						// TODO: Much better logging/ux could be put in here: waiting, timeouts etc...
						storedStatus := servicesStatus[serviceName]

						if isOk != storedStatus {
							log.Info(strings.Join(s.URIs, ","), "name", s.Name, "available", s.Available)
							servicesStatus[serviceName] = isOk
						}
					}

				}
			}
		}

		// After each run, check if all leases are OK
		if wfl.allLeasesOk() {
			cancel() // We're done
			return nil
		}
	}
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

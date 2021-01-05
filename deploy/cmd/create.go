package cmd

import (
	"context"
	"fmt"
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
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadTxCommandFlags(clientCtx, cmd.Flags())
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
				if err = ChainEmitter(ctx, clientCtx, DeploymentDataUpdateHandler(dd), SendManifestHander(clientCtx, dd)); err != nil {
					log.Error("error watching events", err)
				}
				return err
			})

			// Send the deployment creation transaction
			group.Go(func() error {
				if err = TxCreateDeployment(clientCtx, cmd.Flags(), dd); err != nil {
					log.Error("error creating deployment", err)
				}
				return err
			})

			// Wait for the leases to be created and then start polling the provider for service availability
			group.Go(func() error {
				if err = WaitForLeasesAndPollService(clientCtx, dd, cancel); err != nil {
					log.Error("error listening for service", err)
				}
				return err
			})

			return group.Wait()
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

// WaitForLeasesAndPollService waits for leases
func WaitForLeasesAndPollService(clientCtx client.Context, dd *DeploymentData, cancel context.CancelFunc) error {
	log := logger
	pclient := pmodule.AppModuleBasic{}.GetQueryClient(clientCtx)
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
			if dd.ExpectedLeases() {
				for _, l := range dd.Leases() {

					var (
						p   *ptypes.Provider
						err error
					)
					if err := retry.Do(func() error {
						res, err := pclient.Provider(
							context.Background(),
							&ptypes.QueryProviderRequest{Owner: l.Provider},
						)
						if err != nil {
							// TODO: Log retry?
							return err
						}

						p = &res.Provider
						return nil
					}); err != nil {
						cancel()
						return fmt.Errorf("error querying provider: %w", err)
					}

					// TODO: Move to using service status here?
					var ls *ctypes.LeaseStatus
					if err := retry.Do(func() error {
						ls, err = gateway.NewClient().LeaseStatus(context.Background(), p.HostURI, l)
						if err != nil {
							return err
						}
						return nil
					}); err != nil {
						cancel()
						return fmt.Errorf("error querying lease status: %w", err)
					}

					for _, s := range ls.Services {
						// TODO: Much better logging/ux could be put in here: waiting, timeouts etc...
						if s.Available == s.Total {
							log.Info(strings.Join(s.URIs, ","), "name", s.Name, "available", s.Available)
							cancel()
							return nil
						}
					}
				}
			}
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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	"github.com/go-kit/kit/log/term"
	"github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/gateway"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	dmodule "github.com/ovrclk/akash/x/deployment"
	mmodule "github.com/ovrclk/akash/x/market"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmcli "github.com/tendermint/tendermint/libs/cli"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/sync/errgroup"
)

const (
	flagClusterK8s           = "cluster-k8s"
	flagK8sManifestNS        = "k8s-manifest-ns"
	flagGatewayListenAddress = "gateway-listen-address"
)

var (
	errInvalidConfig = errors.New("Invalid configuration")
)

// RunLocalProvider wraps up the Provider cobra command for testing and supplies
// new default values to the flags.
// prev: akashctl provider run --from=foo --cluster-k8s --gateway-listen-address=localhost:39729 --home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863 --keyring-backend test
func RunLocalProvider(clientCtx cosmosclient.Context, chainID, nodeRPC, akashHome, from, gatewayListenAddress string) (sdktest.BufferWriter, error) {
	cmd := runCmd()
	// Flags added because command not being wrapped by the Tendermint's PrepareMainCmd()
	cmd.PersistentFlags().StringP(tmcli.HomeFlag, "", akashHome, "directory for config and data")
	cmd.PersistentFlags().Bool(tmcli.TraceFlag, false, "print out full stack trace on errors")

	args := []string{
		"--cluster-k8s",
		fmt.Sprintf("--%s=%s", flags.FlagChainID, chainID),
		fmt.Sprintf("--%s=%s", flags.FlagNode, nodeRPC),
		fmt.Sprintf("--%s=%s", flags.FlagHome, akashHome),
		fmt.Sprintf("--from=%s", from),
		fmt.Sprintf("--%s=%s", flagGatewayListenAddress, gatewayListenAddress),
		fmt.Sprintf("--%s=%s", flags.FlagKeyringBackend, keyring.BackendTest),
	}
	fmt.Printf("akash provider run args: %v\n", args)

	return clitestutil.ExecTestCLICmd(clientCtx, cmd, args)
}

func runCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "run akash provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForever(func(ctx context.Context) error {
				return doRunCmd(ctx, cmd, args)
			})
		},
	}

	cmd.Flags().String(flags.FlagChainID, "", "The network chain ID")
	if err := viper.BindPFlag(flags.FlagChainID, cmd.Flags().Lookup(flags.FlagChainID)); err != nil {
		return nil
	}

	flags.AddTxFlagsToCmd(cmd)

	cmd.Flags().Bool(flagClusterK8s, false, "Use Kubernetes cluster")
	if err := viper.BindPFlag(flagClusterK8s, cmd.Flags().Lookup(flagClusterK8s)); err != nil {
		return nil
	}

	cmd.Flags().String(flagK8sManifestNS, "lease", "Cluster manifest namespace")
	if err := viper.BindPFlag(flagK8sManifestNS, cmd.Flags().Lookup(flagK8sManifestNS)); err != nil {
		return nil
	}

	cmd.Flags().String(flagGatewayListenAddress, "0.0.0.0:8080", "Gateway listen address")
	if err := viper.BindPFlag(flagGatewayListenAddress, cmd.Flags().Lookup(flagGatewayListenAddress)); err != nil {
		return nil
	}

	return cmd
}

// doRunCmd initializes all of the Provider functionality, hangs, and awaits shutdown signals.
func doRunCmd(ctx context.Context, cmd *cobra.Command, _ []string) error {
	fmt.Printf("SDK GET CLIENT CONTEXT\n")
	cctx := sdkclient.GetClientContextFromCmd(cmd)

	fmt.Printf("clientCtx.From: %q genOnly: %v\n", cctx.From, cctx.GenerateOnly)
	fmt.Printf("READ TX COMMAND FLAGS\n")
	flagSet := cmd.Flags()
	from, _ := flagSet.GetString(flags.FlagFrom)
	fmt.Printf("FLAGSET FROM: %q\n", from)
	addr, key, err := cosmosclient.GetFromFields(cctx.Keyring, from, false)
	fmt.Printf("debugging: %v %q, %v\n", addr, key, err)

	cctx, err = sdkclient.ReadTxCommandFlags(cctx, cmd.Flags())
	if err != nil {
		return err
	}

	txFactory := tx.NewFactoryCLI(cctx, cmd.Flags()).WithTxConfig(cctx.TxConfig).WithAccountRetriever(cctx.AccountRetriever)

	keyname := cctx.GetFromName()
	info, err := txFactory.Keybase().Key(keyname)
	if err != nil {
		return err
	}

	gwaddr := viper.GetString(flagGatewayListenAddress)

	log := openLogger()

	// TODO: actually get the passphrase?
	// passphrase, err := keys.GetPassphrase(fromName)
	aclient := client.NewClient(
		log,
		cctx,
		txFactory,
		info,
		keys.DefaultKeyPass,
		client.NewQueryClient(
			dmodule.AppModuleBasic{}.GetQueryClient(cctx),
			mmodule.AppModuleBasic{}.GetQueryClient(cctx),
			pmodule.AppModuleBasic{}.GetQueryClient(cctx),
		),
	)

	res, err := aclient.Query().Provider(
		context.Background(),
		&ptypes.QueryProviderRequest{Owner: info.GetAddress()},
	)
	if err != nil {
		return err
	}

	pinfo := &res.Provider

	// k8s client creation
	cclient, err := createClusterClient(log, cmd, pinfo.HostURI)
	if err != nil {
		return err
	}

	session := session.New(log, aclient, pinfo)

	if err := cctx.Client.Start(); err != nil {
		return err
	}

	bus := pubsub.NewBus()
	defer bus.Close()

	group, ctx := errgroup.WithContext(ctx)

	service, err := provider.NewService(ctx, session, bus, cclient)
	if err != nil {
		return group.Wait()
	}

	gateway := gateway.NewServer(ctx, log, service, gwaddr)

	group.Go(func() error {
		return events.Publish(ctx, cctx.Client, "provider-cli", bus)
	})

	group.Go(func() error {
		<-service.Done()
		return nil
	})

	group.Go(gateway.ListenAndServe)

	group.Go(func() error {
		<-ctx.Done()
		return gateway.Close()
	})

	return group.Wait()
}

func openLogger() log.Logger {
	// logger with no color output - current debug colors are invisible for me.
	return log.NewTMLoggerWithColorFn(log.NewSyncWriter(os.Stdout), func(_ ...interface{}) term.FgBgColor {
		return term.FgBgColor{}
	})
}

func createClusterClient(log log.Logger, _ *cobra.Command, host string) (cluster.Client, error) {
	if !viper.GetBool(flagClusterK8s) {
		// Condition that there is no Kubernetes API to work with.
		return cluster.NullClient(), nil
	}
	ns := viper.GetString(flagK8sManifestNS)
	if ns == "" {
		return nil, fmt.Errorf("%w: --%s required", errInvalidConfig, flagK8sManifestNS)
	}
	return kube.NewClient(log, host, ns)
}

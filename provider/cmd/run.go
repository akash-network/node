package cmd

import (
	"context"
	"os"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	dmodule "github.com/ovrclk/akash/x/deployment"
	mmodule "github.com/ovrclk/akash/x/market"
	pmodule "github.com/ovrclk/akash/x/provider"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/sync/errgroup"
)

const (
	flagClusterK8s    = "cluster-k8s"
	flagK8sManifestNS = "k8s-manifest-ns"
)

func runCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "run akash provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			return common.RunForever(func(ctx context.Context) error {
				return doRunCmd(ctx, cdc, cmd, args)
			})
		},
	}

	cmd.Flags().Bool(flagClusterK8s, false, "Use Kubernetes cluster")
	cmd.Flags().String(flagK8sManifestNS, "lease", "Cluster manifest namespace")

	return cmd
}

func doRunCmd(ctx context.Context, cdc *codec.Codec, cmd *cobra.Command, _ []string) error {
	cctx := ccontext.NewCLIContext().WithCodec(cdc)

	txbldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

	keyname := cctx.GetFromName()
	info, err := txbldr.Keybase().Get(keyname)
	if err != nil {
		return err
	}

	log := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

	// TODO: actually get the passphrase?
	// passphrase, err := keys.GetPassphrase(fromName)

	aclient := client.NewClient(
		log,
		cctx,
		txbldr,
		info,
		keys.DefaultKeyPass,
		client.NewQueryClient(
			dmodule.AppModuleBasic{}.GetQueryClient(cctx),
			mmodule.AppModuleBasic{}.GetQueryClient(cctx),
			pmodule.AppModuleBasic{}.GetQueryClient(cctx),
		),
	)

	pinfo, err := aclient.Query().Provider(info.GetAddress())
	if err != nil {
		return err
	}

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
		group.Wait()
		return err
	}

	group.Go(func() error {
		return events.Publish(ctx, cctx.Client, "provider-cli", bus)
	})

	group.Go(func() error {
		<-service.Done()
		return nil
	})

	return group.Wait()
}

func createClusterClient(log log.Logger, cmd *cobra.Command, host string) (cluster.Client, error) {
	if val, _ := cmd.Flags().GetBool(flagClusterK8s); !val {
		return cluster.NullClient(), nil
	}
	ns, err := cmd.Flags().GetString(flagK8sManifestNS)
	if err != nil {
		return nil, err
	}
	return kube.NewClient(log, host, ns)
}

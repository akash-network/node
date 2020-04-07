package main

import (
	"context"
	"os"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/keys"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	dmodule "github.com/ovrclk/akash/x/deployment"
	mmodule "github.com/ovrclk/akash/x/market"
	pmodule "github.com/ovrclk/akash/x/provider"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/libs/log"
)

func providerCmd(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "run akash provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx := ccontext.NewCLIContext().WithCodec(cdc)
			ctx := context.Background()

			txbldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			keyname := cctx.GetFromName()
			info, err := txbldr.Keybase().Get(keyname)
			if err != nil {
				return err
			}

			// TODO: actually get the passphrase.
			// passphrase, err := keys.GetPassphrase(fromName)

			aclient := client.NewClient(
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

			log := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

			session := session.New(log, aclient, pinfo)

			bus := pubsub.NewBus()
			defer bus.Close()

			pubdone := make(chan error, 1)

			go func() {
				pubdone <- events.Publish(ctx, cctx.Client, "provider-cli", bus)
			}()

			var cclient cluster.Client

			service, err := provider.NewService(ctx, session, bus, cclient)
			if err != nil {
				return err
			}

			<-service.Done()

			return nil
		},
	}

	cmd.Flags().Bool("cluster-k8s", false, "Use Kubernetes cluster")
	cmd.Flags().String("manifest-ns", "lease", "Cluster manifest namespace")

	return cmd
}

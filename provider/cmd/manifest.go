package cmd

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ovrclk/akash/provider/gateway"
	"github.com/ovrclk/akash/provider/manifest"
	"github.com/ovrclk/akash/sdl"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"
	"github.com/spf13/cobra"
)

func sendManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-manifest <sdl-path>",
		Args:  cobra.ExactArgs(1),
		Short: "Submit manifest to provider(s)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doSendManifest(cmd, args[0])
		},
	}
	mcli.AddBidIDFlags(cmd.Flags())
	mcli.MarkReqBidIDFlags(cmd)
	return cmd
}

func doSendManifest(cmd *cobra.Command, sdlpath string) error {
	cctx := client.GetClientContextFromCmd(cmd)

	sdl, err := sdl.ReadFile(sdlpath)
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	bid, err := mcli.BidIDFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	lid := mtypes.MakeLeaseID(bid)

	pclient := pmodule.AppModuleBasic{}.GetQueryClient(cctx)
	provider, err := pclient.Provider(lid.Provider)
	if err != nil {
		return err
	}

	gclient := gateway.NewClient()

	return gclient.SubmitManifest(
		context.Background(),
		provider.HostURI,
		&manifest.SubmitRequest{
			Deployment: lid.DeploymentID(),
			Manifest:   mani,
		},
	)
}

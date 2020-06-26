package cmd

import (
	"context"

	ccontext "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/spf13/cobra"

	"github.com/ovrclk/akash/provider/gateway"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"
)

func serviceStatusCmd(codec *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-status",
		Short: "get service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doServiceStatus(codec, cmd)
		},
	}

	mcli.AddBidIDFlags(cmd.Flags())
	mcli.MarkReqBidIDFlags(cmd)

	return cmd
}

func doServiceStatus(codec *codec.Codec, cmd *cobra.Command) error {
	cctx := ccontext.NewCLIContext().WithCodec(codec)

	addr, err := mcli.ProviderFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	var svcName string
	if svcName, err = cmd.Flags().GetString("service"); err != nil {
		return err
	}

	pclient := pmodule.AppModuleBasic{}.GetQueryClient(cctx)
	provider, err := pclient.Provider(addr)
	if err != nil {
		return err
	}

	gclient := gateway.NewClient()

	bid, err := mcli.BidIDFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	lid := mtypes.MakeLeaseID(bid)

	result, err := gclient.ServiceStatus(context.Background(), provider.HostURI, lid, svcName)
	if err != nil {
		return err
	}

	if err = cctx.PrintOutput(result); err != nil {
		return err
	}

	return nil
}

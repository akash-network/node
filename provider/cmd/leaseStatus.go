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

func leaseStatusCmd(codec *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease-status",
		Short: "get lease status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLeaseStatus(codec, cmd)
		},
	}

	mcli.AddBidIDFlags(cmd.Flags())
	mcli.MarkReqBidIDFlags(cmd)

	return cmd
}

func doLeaseStatus(codec *codec.Codec, cmd *cobra.Command) error {
	cctx := ccontext.NewCLIContext().WithCodec(codec)

	addr, err := mcli.ProviderFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
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

	result, err := gclient.LeaseStatus(context.Background(), provider.HostURI, lid)
	if err != nil {
		return err
	}

	if err = cctx.PrintOutput(result); err != nil {
		return err
	}

	return nil
}

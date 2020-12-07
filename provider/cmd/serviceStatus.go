package cmd

import (
	"context"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	cmdcommon "github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/provider/gateway"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

const FlagService = "service"

func serviceStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "service-status",
		Short:        "get service status",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doServiceStatus(cmd)
		},
	}

	mcli.AddBidIDFlags(cmd.Flags())
	mcli.MarkReqBidIDFlags(cmd)
	cmd.Flags().String(FlagService, "", "name of service to query")
	if err := cmd.MarkFlagRequired(FlagService); err != nil {
		return nil
	}

	return cmd
}

func doServiceStatus(cmd *cobra.Command) error {
	cctx := client.GetClientContextFromCmd(cmd)

	addr, err := mcli.ProviderFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	var svcName string
	if svcName, err = cmd.Flags().GetString(FlagService); err != nil {
		return err
	}

	pclient := pmodule.AppModuleBasic{}.GetQueryClient(cctx)
	res, err := pclient.Provider(context.Background(), &ptypes.QueryProviderRequest{Owner: addr.String()})
	if err != nil {
		return err
	}

	provider := &res.Provider
	gclient := gateway.NewClient()

	bid, err := mcli.BidIDFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	lid := mtypes.MakeLeaseID(bid)

	result, err := gclient.ServiceStatus(context.Background(), provider.HostURI, lid, svcName)
	if err != nil {
		return showErrorToUser(err)
	}

	return cmdcommon.PrintJSON(cctx, result)
}

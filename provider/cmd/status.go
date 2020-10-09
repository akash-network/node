package cmd

import (
	"context"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	cmdcommon "github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/provider/gateway"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "get provider status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doStatus(cmd)
		},
	}

	mcli.AddProviderFlag(cmd.Flags())
	mcli.MarkReqProviderFlag(cmd)

	return cmd
}

func doStatus(cmd *cobra.Command) error {
	cctx := client.GetClientContextFromCmd(cmd)

	addr, err := mcli.ProviderFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	pclient := pmodule.AppModuleBasic{}.GetQueryClient(cctx)

	res, err := pclient.Provider(context.Background(), &ptypes.QueryProviderRequest{Owner: addr})
	if err != nil {
		return err
	}

	provider := &res.Provider
	gclient := gateway.NewClient()

	result, err := gclient.Status(context.Background(), provider.HostURI)
	if err != nil {
		return err
	}

	return cmdcommon.PrintJSONStdout(result)
}

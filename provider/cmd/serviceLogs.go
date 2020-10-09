package cmd

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cmdcommon "github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/provider/gateway"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
	pmodule "github.com/ovrclk/akash/x/provider"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

func serviceLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-logs",
		Short: "get service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doServiceLogs(cmd)
		},
	}

	mcli.AddBidIDFlags(cmd.Flags())
	mcli.MarkReqBidIDFlags(cmd)

	cmd.Flags().String("service", "", "")
	_ = cmd.MarkFlagRequired("service")

	cmd.Flags().BoolP("follow", "f", false, "Specify if the logs should be streamed. Defaults to false")
	cmd.Flags().Int64P("tail", "t", -1, "The number of lines from the end of the logs to show. Defaults to -1")
	cmd.Flags().String("format", "text", "Output format text|json. Defaults to text")
	return cmd
}

func doServiceLogs(cmd *cobra.Command) error {
	cctx := client.GetClientContextFromCmd(cmd)

	addr, err := mcli.ProviderFromFlagsWithoutCtx(cmd.Flags())
	if err != nil {
		return err
	}

	svcName, err := cmd.Flags().GetString("service")
	if err != nil {
		return err
	}

	outputFormat, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	if outputFormat != "text" && outputFormat != "json" {
		return errors.Errorf("invalid output format %s", outputFormat)
	}

	pclient := pmodule.AppModuleBasic{}.GetQueryClient(cctx)
	res, err := pclient.Provider(context.Background(), &ptypes.QueryProviderRequest{Owner: addr})
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

	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return err
	}

	tailLines, err := cmd.Flags().GetInt64("tail")
	if err != nil {
		return err
	}

	if tailLines < -1 {
		return errors.Errorf("tail flag supplied with invalid value. must be >= -1")
	}

	result, err := gclient.ServiceLogs(context.Background(), provider.HostURI, lid, svcName, follow, tailLines)
	if err != nil {
		return err
	}

	printFn := func(msg gateway.ServiceLogMessage) error {
		fmt.Printf("[%s] %s\n", msg.Name, msg.Message)
		return nil
	}

	if outputFormat == "json" {
		printFn = func(msg gateway.ServiceLogMessage) error {
			return cmdcommon.PrintJSONStdout(msg)
		}
	}

	for res := range result.Stream {
		err = printFn(res)
		if err != nil {
			return err
		}
	}

	return nil
}

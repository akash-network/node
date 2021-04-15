package cmd

import (
	"crypto/tls"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	akashclient "github.com/ovrclk/akash/client"
	cmdcommon "github.com/ovrclk/akash/cmd/common"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
)

func leaseEventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "lease-events",
		Short:        "get lease events",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLeaseEvents(cmd)
		},
	}

	addServiceFlags(cmd)
	cmd.Flags().BoolP("follow", "f", false, "Specify if the logs should be streamed. Defaults to false")
	cmd.Flags().Int64P("tail", "t", -1, "The number of lines from the end of the logs to show. Defaults to -1")

	return cmd
}

func doLeaseEvents(cmd *cobra.Command) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	prov, err := providerFromFlags(cmd.Flags())
	if err != nil {
		return err
	}

	bid, err := mcli.BidIDFromFlagsForOwner(cmd.Flags(), cctx.FromAddress)
	if err != nil {
		return err
	}

	svcs, err := cmd.Flags().GetString(FlagService)
	if err != nil {
		return err
	}

	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return err
	}

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		return err
	}

	gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
	if err != nil {
		return err
	}

	result, err := gclient.LeaseEvents(cmd.Context(), bid.LeaseID(), svcs, follow)
	if err != nil {
		return showErrorToUser(err)
	}

	for res := range result.Stream {
		if err = cmdcommon.PrintJSON(cctx, res); err != nil {
			break
		}
	}

	select {
	case msg, ok := <-result.OnClose:
		if ok && msg != "" {
			_ = cctx.PrintString(msg + "\n")
		}
	default:
	}

	return nil
}

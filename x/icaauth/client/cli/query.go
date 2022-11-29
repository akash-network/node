package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	typesv1beta2 "github.com/ovrclk/akash/x/icaauth/types/v1beta2"
)

// GetQueryCmd creates and returns the icaauth query command
func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        typesv1beta2.ModuleName,
		Short:                      "Querying commands for the icaatuh module",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(getInterchainAccountCmd())

	return cmd
}

func getInterchainAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "icaccounts [connection-id] [owner-account]",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			queryClient := typesv1beta2.NewQueryClient(clientCtx)
			res, err := queryClient.InterchainAccountFromAddress(
				cmd.Context(),
				&typesv1beta2.QueryInterchainAccountFromAddressRequest{
					Owner:        args[1],
					ConnectionId: args[0],
				},
			)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

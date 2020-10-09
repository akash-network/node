package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/spf13/cobra"
)

// GetQueryCmd returns the transaction commands for the provider module
func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Provider query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetProviders(),
		cmdGetProvider(),
	)

	return cmd
}

func cmdGetProviders() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadQueryCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			pageReq, err := client.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryProvidersRequest{
				Pagination: pageReq,
			}

			res, err := queryClient.Providers(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintOutput(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	flags.AddPaginationFlagsToCmd(cmd, "providers")

	return cmd
}

func cmdGetProvider() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [address]",
		Short: "Query provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadQueryCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			owner, err := sdk.AccAddressFromBech32(args[0])
			if err != nil {
				return err
			}

			res, err := queryClient.Provider(context.Background(), &types.QueryProviderRequest{Owner: owner.String()})
			if err != nil {
				return err
			}

			return clientCtx.PrintOutput(&res.Provider)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

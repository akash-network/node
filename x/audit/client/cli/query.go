package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"github.com/ovrclk/akash/x/audit/types"
)

func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Audit query commands",
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
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			pageReq, err := client.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryAllProvidersAttributesRequest{
				Pagination: pageReq,
			}

			res, err := queryClient.AllProvidersAttributes(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	flags.AddPaginationFlagsToCmd(cmd, "providers")

	return cmd
}

func cmdGetProvider() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [owner address] [auditor address]",
		Short: "Query provider",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			owner, err := sdk.AccAddressFromBech32(args[0])
			if err != nil {
				return err
			}

			var res *types.QueryProvidersResponse
			if len(args) == 1 {
				res, err = queryClient.ProviderAttributes(context.Background(),
					&types.QueryProviderAttributesRequest{
						Owner: owner.String(),
					},
				)
			} else {
				var auditor sdk.AccAddress
				if auditor, err = sdk.AccAddressFromBech32(args[1]); err != nil {
					return err
				}

				res, err = queryClient.ProviderAuditorAttributes(context.Background(),
					&types.QueryProviderAuditorRequest{
						Auditor: auditor.String(),
						Owner:   owner.String(),
					},
				)
			}

			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

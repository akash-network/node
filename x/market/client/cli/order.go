package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

func cmdGetOrders() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all orders",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			ofilters, err := OrderFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := client.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryOrdersRequest{
				Filters:    ofilters,
				Pagination: pageReq,
			}

			res, err := queryClient.Orders(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	flags.AddPaginationFlagsToCmd(cmd, "orders")
	AddOrderFilterFlags(cmd.Flags())
	return cmd
}

func cmdGetOrder() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query order",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := queryClient.Order(context.Background(), &types.QueryOrderRequest{ID: id})
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(&res.Order)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	AddOrderIDFlags(cmd.Flags())
	MarkReqOrderIDFlags(cmd)

	return cmd
}

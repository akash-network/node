package cli

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	aclient "github.com/akash-network/node/client"
	clientutils "github.com/akash-network/node/client"
)

func cmdGetOrders() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all orders",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			qq, err := aclient.DiscoverQueryClient(ctx, cctx)
			if err != nil {
				return err
			}

			ofilters, err := OrderFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := clientutils.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryOrdersRequest{
				Filters:    ofilters,
				Pagination: pageReq,
			}

			res, err := qq.Orders(cmd.Context(), params)
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
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
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			qq, err := aclient.DiscoverQueryClient(ctx, cctx)
			if err != nil {
				return err
			}

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := qq.Order(cmd.Context(), &types.QueryOrderRequest{ID: id})
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(&res.Order)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	AddOrderIDFlags(cmd.Flags())
	MarkReqOrderIDFlags(cmd)

	return cmd
}

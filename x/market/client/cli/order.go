package cli

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/node/market/v1beta5"

	aclient "pkg.akt.dev/akashd/client"
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

			ofilters, err := cli.OrderFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &v1beta5.QueryOrdersRequest{
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

	cli.AddQueryFlagsToCmd(cmd)
	cli.AddPaginationFlagsToCmd(cmd, "orders")
	cli.AddOrderFilterFlags(cmd.Flags())
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

			id, err := cli.OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := qq.Order(cmd.Context(), &v1beta5.QueryOrderRequest{ID: id})
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(&res.Order)
		},
	}

	cli.AddQueryFlagsToCmd(cmd)
	cli.AddOrderIDFlags(cmd.Flags())
	cli.MarkReqOrderIDFlags(cmd)

	return cmd
}

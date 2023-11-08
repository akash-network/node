package cli

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"

	aclient "github.com/akash-network/node/client"
)

func cmdGetBids() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all bids",
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

			bfilters, err := BidFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &types.QueryBidsRequest{
				Filters:    bfilters,
				Pagination: pageReq,
			}

			res, err := qq.Bids(cmd.Context(), params)
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	flags.AddPaginationFlagsToCmd(cmd, "bids")
	AddBidFilterFlags(cmd.Flags())

	return cmd
}

func cmdGetBid() *cobra.Command {
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

			bidID, err := BidIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := qq.Bid(cmd.Context(), &types.QueryBidRequest{ID: bidID})
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)
	AddQueryBidIDFlags(cmd.Flags())
	MarkReqBidIDFlags(cmd)

	return cmd
}

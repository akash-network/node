package cli

import (
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"pkg.akt.dev/go/cli"
	v1 "pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/node/market/v1beta5"

	aclient "pkg.akt.dev/akashd/client"
)

func cmdGetLeases() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all leases",
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

			lfilters, err := cli.LeaseFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &v1beta5.QueryLeasesRequest{
				Filters:    lfilters,
				Pagination: pageReq,
			}

			res, err := qq.Leases(cmd.Context(), params)
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
		},
	}

	cli.AddQueryFlagsToCmd(cmd)
	cli.AddPaginationFlagsToCmd(cmd, "leases")
	cli.AddLeaseFilterFlags(cmd.Flags())

	return cmd
}

func cmdGetLease() *cobra.Command {
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

			bidID, err := cli.BidIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := qq.Lease(cmd.Context(), &v1beta5.QueryLeaseRequest{ID: v1.MakeLeaseID(bidID)})
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
		},
	}

	cli.AddQueryFlagsToCmd(cmd)
	cli.AddQueryBidIDFlags(cmd.Flags())
	cli.MarkReqBidIDFlags(cmd)

	return cmd
}

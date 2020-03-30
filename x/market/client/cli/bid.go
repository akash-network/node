package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/market/query"
	"github.com/spf13/cobra"
)

func cmdGetBids(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all bids",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			id, err := BidFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			var obj query.Bids

			if id.Owner.Empty() && id.State == 100 {
				obj, err = query.NewClient(ctx, key).Bids()
			} else {
				obj, err = query.NewClient(ctx, key).FilterBids(id)
			}

			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
	AddBidFilterFlags(cmd.Flags())
	return cmd
}

func cmdGetBid(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query order",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			bid, err := BidIDFromFlagsWithoutCtx(cmd.Flags())
			if err != nil {
				return err
			}

			obj, err := query.NewClient(ctx, key).Bid(bid)
			if err != nil {
				return err
			}

			return ctx.PrintOutput(obj)
		},
	}
	AddQueryBidIDFlags(cmd.Flags())
	MarkReqBidIDFlags(cmd)
	return cmd
}

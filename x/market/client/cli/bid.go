package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/market/query"
	"github.com/spf13/cobra"
)

func cmdGetBids(key string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Query for all bids",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			obj, err := query.NewClient(ctx, key).Bids()
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
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

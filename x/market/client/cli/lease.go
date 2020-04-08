package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/market/query"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

func cmdGetLeases(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all leases",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			lfilters, err := LeaseFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			obj, err := query.NewClient(ctx, key).Leases(lfilters)
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
	AddLeaseFilterFlags(cmd.Flags())
	return cmd
}

func cmdGetLease(key string, cdc *codec.Codec) *cobra.Command {
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

			obj, err := query.NewClient(ctx, key).Lease(types.MakeLeaseID(bid))
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

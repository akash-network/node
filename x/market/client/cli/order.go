package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"

	"github.com/ovrclk/akash/x/market/query"
	xtypes "github.com/ovrclk/akash/x/types"

	"github.com/spf13/cobra"
)

func cmdGetOrders(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all orders",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			ofilters, err := OrderFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			obj, err := query.NewClient(ctx, key).Orders(ofilters)
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
	AddOrderFilterFlags(cmd.Flags())
	xtypes.AddPaginationFlags(cmd.Flags())
	return cmd
}

func cmdGetOrder(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query order",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			obj, err := query.NewClient(ctx, key).Order(id)
			if err != nil {
				return err
			}

			return ctx.PrintOutput(obj)
		},
	}
	AddOrderIDFlags(cmd.Flags())
	MarkReqOrderIDFlags(cmd)
	return cmd
}

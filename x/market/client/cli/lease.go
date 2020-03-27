package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/market/query"
	"github.com/spf13/cobra"
)

func cmdGetLeases(key string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Query for all leases",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			obj, err := query.NewClient(ctx, key).Leases()
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
}

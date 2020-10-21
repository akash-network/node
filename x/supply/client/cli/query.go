package cli

import (
	"github.com/ovrclk/akash/x/supply/query"
	"github.com/ovrclk/akash/x/supply/types"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/spf13/cobra"
)

// GetCirculatingSupply returns circulation supply
func GetCirculatingSupply(cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "circulating",
		Short: "Display circulating (ie non-vesting) token supply",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			res, err := query.NewClient(ctx, types.ModuleName).CirculatingSupply()
			if err != nil {
				return err
			}

			return ctx.PrintOutput(res)
		},
	}

	return flags.GetCommands(cmd)[0]
}

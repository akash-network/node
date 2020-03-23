package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/query"
	"github.com/spf13/cobra"
)

// GetQueryCmd returns the transaction commands for the provider module
func GetQueryCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "providers",
		Short:                      "Provider query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdGetProviders(key, cdc),
		// cmdGetProvider(key, cdc),
	)...)

	return cmd
}

func cmdGetProviders(key string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:  "provider [<address>]",
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			if len(args) == 1 {
				id, err := sdk.AccAddressFromBech32(args[0])
				if err != nil {
					return err
				}
				obj, err := query.NewClient(ctx, key).Provider(id)
				if err != nil {
					return err
				}
				return ctx.PrintOutput(obj)
			}

			obj, err := query.NewClient(ctx, key).Providers()
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
}

// func cmdGetProvider(key string, cdc *codec.Codec) *cobra.Command {
// 	return &cobra.Command{
// 		Use:  "provider <address>",
// 		Args: cobra.ExactArgs(1),
// 		RunE: func(cmd *cobra.Command, args []string) error {
// 			ctx := context.NewCLIContext().WithCodec(cdc)

// 			id, err := sdk.AccAddressFromBech32(args[0])
// 			if err != nil {
// 				return err
// 			}
// 			obj, err := query.NewClient(ctx, key).Provider(id)
// 			if err != nil {
// 				return err
// 			}
// 			return ctx.PrintOutput(obj)
// 		},
// 	}
// }

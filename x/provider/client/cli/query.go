package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/provider/query"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/spf13/cobra"
)

// GetQueryCmd returns the transaction commands for the provider module
func GetQueryCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Provider query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdGetProviders(key, cdc),
	)...)

	return cmd
}

func cmdGetProviders(key string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use: "providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			obj, err := query.NewClient(ctx, key).Providers()
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
}

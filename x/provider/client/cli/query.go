package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/provider/query"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/spf13/cobra"
)

func GetQueryCmd(key string, cdc *codec.Codec) *cobra.Command {

	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Provider query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(client.GetCommands(
		cmdGetProviders(key, cdc),
	)...)

	return cmd
}

func cmdGetProviders(key string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use: "providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			var obj query.Providers
			ctx := context.NewCLIContext().WithCodec(cdc)
			buf, _, err := ctx.QueryWithData(fmt.Sprintf("custom/%s/%s", key, query.ProvidersPath()), nil)
			if err != nil {
				return err
			}
			if err := cdc.UnmarshalJSON(buf, &obj); err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
}

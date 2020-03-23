package cli

import (
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ovrclk/akash/x/provider/config"
	"github.com/ovrclk/akash/x/provider/types"
	"github.com/spf13/cobra"
)

// GetTxCmd returns the transaction commands for provider module
func GetTxCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Deployment transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(flags.PostCommands(
		cmdCreate(key, cdc),
		cmdUpdate(key, cdc),
	)...)
	return cmd
}

func cmdCreate(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [config-file]",
		Short: "Create provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			cfg, err := config.ReadConfigPath(args[0])
			if err != nil {
				return err
			}

			msg := types.MsgCreate{
				Owner:      ctx.GetFromAddress(),
				HostURI:    cfg.Host,
				Attributes: cfg.GetAttributes(),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}

	return cmd
}

func cmdUpdate(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [config-file]",
		Short: "Update provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(utils.GetTxEncoder(cdc))

			cfg, err := config.ReadConfigPath(args[0])
			if err != nil {
				return err
			}

			msg := types.MsgUpdate{
				Owner:      ctx.GetFromAddress(),
				HostURI:    cfg.Host,
				Attributes: cfg.GetAttributes(),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}

	return cmd
}

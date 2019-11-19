package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/client/utils"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/x/deployment/types"

	"github.com/spf13/cobra"
)

func GetTxCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Deployment transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(client.PostCommands(
		cmdCreate(key, cdc),
		cmdUpdate(key, cdc),
		cmdClose(key, cdc),
	)...)
	return cmd
}

func cmdCreate(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [sdl-file]",
		Short: "Create deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI().WithTxEncoder(utils.GetTxEncoder(cdc))

			sdl, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			groups, err := sdl.DeploymentGroups()
			if err != nil {
				return err
			}

			msg := types.MsgCreate{
				Owner: ctx.GetFromAddress(),
				// Version:  []byte{0x1, 0x2},
				Groups: make([]types.GroupSpec, 0, len(groups)),
			}

			for _, group := range groups {
				msg.Groups = append(msg.Groups, *group)
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}

	return cmd
}

func cmdClose(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: "Close deployment",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI().WithTxEncoder(utils.GetTxEncoder(cdc))

			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			msg := types.MsgClose{
				ID: id,
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddDeploymentIDFlags(cmd.Flags())
	return cmd
}

func cmdUpdate(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [sdl-file]",
		Short: "update deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI().WithTxEncoder(utils.GetTxEncoder(cdc))

			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.GetFromAddress().String())
			if err != nil {
				return err
			}

			msg := types.MsgUpdate{
				ID: id,
			}

			return utils.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddDeploymentIDFlags(cmd.Flags())
	return cmd
}

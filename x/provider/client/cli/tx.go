package cli

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	types "github.com/akash-network/akash-api/go/node/provider/v1beta3"

	aclient "github.com/akash-network/node/client"
	"github.com/akash-network/node/x/provider/config"
)

// GetTxCmd returns the transaction commands for provider module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Provider transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}
	cmd.AddCommand(
		cmdCreate(key),
		cmdUpdate(key),
	)
	return cmd
}

func cmdCreate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [config-file]",
		Short: fmt.Sprintf("Create a %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, cmd.Flags())
			if err != nil {
				return err
			}

			// TODO: enable reading files with non-local URIs
			cfg, err := config.ReadConfigPath(args[0])
			if err != nil {
				err = fmt.Errorf("%w: ReadConfigPath err: %q", err, args[0])
				return err
			}

			msg := &types.MsgCreateProvider{
				Owner:      cctx.GetFromAddress().String(),
				HostURI:    cfg.Host,
				Info:       cfg.Info,
				Attributes: cfg.GetAttributes(),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

func cmdUpdate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [config-file]",
		Short: fmt.Sprintf("Update %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, cmd.Flags())
			if err != nil {
				return err
			}

			cfg, err := config.ReadConfigPath(args[0])
			if err != nil {
				return err
			}

			msg := &types.MsgUpdateProvider{
				Owner:      cctx.GetFromAddress().String(),
				HostURI:    cfg.Host,
				Info:       cfg.Info,
				Attributes: cfg.GetAttributes(),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

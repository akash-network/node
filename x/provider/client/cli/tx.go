package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/provider/config"
	types "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

// GetTxCmd returns the transaction commands for provider module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Provider transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
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
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			// TODO: enable reading files with non-local URIs
			cfg, err := config.ReadConfigPath(args[0])
			if err != nil {
				err = errors.Wrapf(err, "ReadConfigPath err: %q", args[0])
				return err
			}

			msg := &types.MsgCreateProvider{
				Owner:      clientCtx.GetFromAddress().String(),
				HostURI:    cfg.Host,
				Info:       cfg.Info,
				Attributes: cfg.GetAttributes(),
				JWTHostURI: cfg.JWTHost,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
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
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			cfg, err := config.ReadConfigPath(args[0])
			if err != nil {
				return err
			}

			msg := &types.MsgUpdateProvider{
				Owner:      clientCtx.GetFromAddress().String(),
				HostURI:    cfg.Host,
				Info:       cfg.Info,
				Attributes: cfg.GetAttributes(),
				JWTHostURI: cfg.JWTHost,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)

	return cmd
}

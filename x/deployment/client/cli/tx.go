package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"

	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/x/deployment/types"

	"github.com/spf13/cobra"
)

// GetTxCmd returns the transaction commands for this module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Deployment transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		cmdCreate(key),
		cmdUpdate(key),
		cmdClose(key),
		cmdGroupClose(key),
	)
	return cmd
}

func cmdCreate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [sdl-file]",
		Short: fmt.Sprintf("Create %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}

			groups, err := sdlManifest.DeploymentGroups()
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), clientCtx.GetFromAddress().String())
			if err != nil {
				return err
			}

			// Default DSeq to the current block height
			if id.DSeq == 0 {
				if id.DSeq, err = currentBlockHeight(clientCtx); err != nil {
					return err
				}
			}

			version, err := sdl.Version(sdlManifest)
			if err != nil {
				return err
			}

			msg := &types.MsgCreateDeployment{
				ID:      id,
				Version: version,
				Groups:  make([]types.GroupSpec, 0, len(groups)),
			}

			for _, group := range groups {
				msg.Groups = append(msg.Groups, *group)
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func cmdClose(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close %s", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), clientCtx.GetFromAddress().String())
			if err != nil {
				return err
			}

			msg := &types.MsgCloseDeployment{ID: id}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func cmdUpdate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [sdl-file]",
		Short: fmt.Sprintf("update %s", key),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags(), clientCtx.GetFromAddress().String())
			if err != nil {
				return err
			}

			sdlManifest, err := sdl.ReadFile(args[0])
			if err != nil {
				return err
			}
			groups, err := sdlManifest.DeploymentGroups()
			if err != nil {
				return err
			}

			version, err := sdl.Version(sdlManifest)
			if err != nil {
				return err
			}

			msg := &types.MsgUpdateDeployment{
				ID:      id,
				Version: version,
				Groups:  make([]types.GroupSpec, 0, len(groups)),
			}

			for _, group := range groups {
				msg.Groups = append(msg.Groups, *group)
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())

	return cmd
}

func cmdGroupClose(_ string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "group-close",
		Short:   "close a Deployment's specific Group",
		Example: "akashctl tx deployment group-close --owner=[Account Address] --dseq=[uint64] --gseq=[uint32]",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCloseGroup{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)

	return cmd
}

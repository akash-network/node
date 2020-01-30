package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/deployment/query"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/spf13/cobra"
)

func GetQueryCmd(key string, cdc *codec.Codec) *cobra.Command {

	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Deployment query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdDeployments(key, cdc),
		cmdDeployment(key, cdc),
	)...)

	return cmd
}

func cmdDeployments(key string, cdc *codec.Codec) *cobra.Command {
	return &cobra.Command{
		Use:  "deployments",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			obj, err := query.NewClient(ctx, key).Deployments()
			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
}

func cmdDeployment(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Query deployment",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			id, err := DeploymentIDFromFlags(cmd.Flags(), ctx.FromAddress.String())
			if err != nil {
				return err
			}

			obj, err := query.NewClient(ctx, key).Deployment(id)
			if err != nil {
				return err
			}

			return ctx.PrintOutput(obj)
		},
	}
	AddDeploymentIDFlags(cmd.Flags())
	return cmd
}

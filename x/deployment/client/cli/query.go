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

// GetQueryCmd returns the query commands for the deployment module
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
		getGroupCmd(key, cdc),
	)...)

	return cmd
}

func cmdDeployments(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all deployments",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			id, err := DepFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			var obj query.Deployments

			if id.Owner.Empty() && id.State == 100 {
				obj, err = query.NewClient(ctx, key).Deployments()
			} else {
				obj, err = query.NewClient(ctx, key).FilterDeployments(id)
			}

			if err != nil {
				return err
			}
			return ctx.PrintOutput(obj)
		},
	}
	AddDeploymentFilterFlags(cmd.Flags())
	return cmd
}

func cmdDeployment(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
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
	MarkReqDeploymentIDFlags(cmd)
	return cmd
}

func getGroupCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "group",
		Short:                      "Deployment group query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdGetGroup(key, cdc),
	)...)

	return cmd
}

func cmdGetGroup(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query group of deployment",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)

			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			obj, err := query.NewClient(ctx, key).Group(id)
			if err != nil {
				return err
			}

			return ctx.PrintOutput(obj)
		},
	}
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)
	return cmd
}

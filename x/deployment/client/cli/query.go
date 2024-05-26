package cli

import (
	"context"

	"github.com/spf13/cobra"

	sdkclient "github.com/cosmos/cosmos-sdk/client"

	"pkg.akt.dev/go/cli"

	"pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/deployment/v1beta4"

	aclient "pkg.akt.dev/akashd/client"
)

// GetQueryCmd returns the query commands for the deployment module
func GetQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        v1.ModuleName,
		Short:                      "Deployment query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		cmdDeployments(),
		cmdDeployment(),
		getGroupCmd(),
	)

	return cmd
}

func cmdDeployments() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Query for all deployments",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			qq, err := aclient.DiscoverQueryClient(ctx, cctx)
			if err != nil {
				return err
			}

			dfilters, err := DepFiltersFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			pageReq, err := sdkclient.ReadPageRequest(cmd.Flags())
			if err != nil {
				return err
			}

			params := &v1beta4.QueryDeploymentsRequest{
				Filters:    dfilters,
				Pagination: pageReq,
			}

			res, err := qq.Deployments(context.Background(), params)
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
		},
	}

	cli.AddQueryFlagsToCmd(cmd)
	cli.AddPaginationFlagsToCmd(cmd, "deployments")
	AddDeploymentFilterFlags(cmd.Flags())

	return cmd
}

func cmdDeployment() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query deployment",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			qq, err := aclient.DiscoverQueryClient(ctx, cctx)
			if err != nil {
				return err
			}

			id, err := DeploymentIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := qq.Deployment(context.Background(), &v1beta4.QueryDeploymentRequest{ID: id})
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(res)
		},
	}

	cli.AddQueryFlagsToCmd(cmd)
	AddDeploymentIDFlags(cmd.Flags())
	MarkReqDeploymentIDFlags(cmd)

	return cmd
}

func getGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "group",
		Short:                      "Deployment group query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetGroup(),
	)

	return cmd
}

func cmdGetGroup() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Query group of deployment",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			qq, err := aclient.DiscoverQueryClient(ctx, cctx)
			if err != nil {
				return err
			}

			id, err := GroupIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			res, err := qq.Group(cmd.Context(), &v1beta4.QueryGroupRequest{ID: id})
			if err != nil {
				return err
			}

			return qq.ClientContext().PrintProto(&res.Group)
		},
	}

	cli.AddQueryFlagsToCmd(cmd)
	AddGroupIDFlags(cmd.Flags())
	MarkReqGroupIDFlags(cmd)

	return cmd
}

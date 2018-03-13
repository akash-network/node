package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func queryDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "query deployment",
		RunE:  context.WithContext(context.RequireNode(doQueryDeploymentCommand)),
	}

	return cmd
}

func doQueryDeploymentCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.DeploymentPath
	if len(args) > 0 {
		structure := new(types.Deployment)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.Deployments)
		return doQuery(ctx, path, structure)
	}
}

package query

import (
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
)

func queryDeploymentOrderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deploymentorder",
		Short: "query deployment order",
		RunE:  context.WithContext(context.RequireNode(doQueryDeploymentOrderCommand)),
	}

	return cmd
}

func doQueryDeploymentOrderCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.DeploymentOrderPath
	if len(args) > 0 {
		structure := new(types.DeploymentOrder)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.DeploymentOrders)
		return doQuery(ctx, path, structure)
	}
}

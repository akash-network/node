package query

import (
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
)

func queryDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "query deployment",
		Args:  cobra.ExactArgs(1),
		RunE:  context.WithContext(context.RequireNode(doQueryDeploymentCommand)),
	}

	return cmd
}

func doQueryDeploymentCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	structure := new(types.Deployment)
	account := args[0]
	path := state.DeploymentPath + account
	doQuery(ctx, path, structure)
	return nil
}

package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/keys"
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
	if len(args) == 0 {
		handleMessage(ctx.QueryClient().Deployments(ctx.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseDeploymentPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(ctx.QueryClient().Deployment(ctx.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

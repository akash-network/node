package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "query deployment",
		RunE:  session.WithSession(session.RequireNode(doQueryDeploymentCommand)),
	}

	return cmd
}

func doQueryDeploymentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		handleMessage(session.QueryClient().Deployments(session.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseDeploymentPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Deployment(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

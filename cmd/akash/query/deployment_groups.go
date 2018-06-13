package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryDeploymentGroupCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment-group [deployment-group ...]",
		Short: "query deployment groups",
		RunE:  session.WithSession(session.RequireNode(doQueryDeploymentGroupCommand)),
	}

	return cmd
}

func doQueryDeploymentGroupCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return handleMessage(session.QueryClient().DeploymentGroups(session.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseGroupPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().DeploymentGroup(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryDeploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment <deployment>...",
		Short: "query deployment",
		RunE:  session.WithSession(session.RequireNode(doQueryDeploymentCommand)),
	}
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	return cmd
}

func doQueryDeploymentCommand(session session.Session, cmd *cobra.Command, args []string) error {

	//depIds := args

	if len(args) == 0 {
		_, info, err := session.Signer()
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().TenantDeployments(session.Ctx(), info.GetPubKey().Address().Bytes())); err != nil {
			return err
		}
		return nil
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

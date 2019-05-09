package deployment

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func closeCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "close <deployment-id>",
		Short: "close a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(close))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	return cmd
}

func close(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	deployment, err := keys.ParseDeploymentPath(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseDeployment{
		Deployment: deployment.ID(),
		Reason:     types.TxCloseDeployment_TENANT_CLOSE,
	})

	if err != nil {
		session.Log().Error("error sending tx", "error", err)
		return err
	}

	fmt.Println("Closing deployment")
	return nil
}

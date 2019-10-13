package deployment

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/dsky"
	"github.com/spf13/cobra"
)

func CloseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <deployment-id>",
		Short: "close a deployment",
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(close))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	return cmd
}

func close(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	var id string
	if len(args) > 0 {
		id = args[0]
	}

	id = session.Mode().Ask().StringVar(id, "Deployment ID (required): ", true)
	if len(id) == 0 {
		return errors.NewArgumentError("deployment:id")
	}

	session.Mode().Printer().Log().WithAction(dsky.LogActionWait).Warn("request close deployment")
	if err != nil {
		return err
	}
	deployment, err := keys.ParseDeploymentPath(id)
	if err != nil {
		return err
	}

	info, err := txclient.BroadcastTxCommit(&types.TxCloseDeployment{
		Deployment: deployment.ID(),
		Reason:     types.TxCloseDeployment_TENANT_CLOSE,
	})
	session.Mode().Printer().Log().WithAction(dsky.LogActionDone).Info("deployment closed")

	data := session.Mode().Printer().NewSection("Close Deployment").NewData().WithTag("raw", info)
	data.
		Add("Deployment ID", deployment.ID()).
		Add("Reason", types.TxCloseDeployment_ReasonCode_name[int32(types.TxCloseDeployment_TENANT_CLOSE)]).
		Add("Height", info.Height).
		Add("Hash", X(info.Hash))

	if err != nil {
		session.Mode().Printer().Log().Error("error sending tx")
		return err
	}

	return session.Mode().Printer().Flush()
	return nil
}

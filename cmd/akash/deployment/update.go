package deployment

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func updateDeploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <manifest> <deployment>",
		Short: "update a deployment (*EXPERIMENTAL*)",
		Args:  cobra.ExactArgs(2),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(updateDeployment))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func updateDeployment(session session.Session, cmd *cobra.Command, args []string) error {

	fmt.Println(`WARNING: this command is experimental and limited.

	It is currently only possible to make small changes to your deployment.

	Resources within a datacenter must remain the same.  You can change ports
	and images;add and remove services; etc... so long as the overall
	infrastructure requirements do not change.
	`)

	signer, _, err := session.Signer()
	if err != nil {
		return err
	}

	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	daddr, err := keys.ParseDeploymentPath(args[1])
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	if err := manifestValidateResources(session, mani, daddr); err != nil {
		return err
	}

	hash, err := manifest.Hash(mani)
	if err != nil {
		return err
	}

	fmt.Println("updating deployment...")

	_, err = txclient.BroadcastTxCommit(&types.TxUpdateDeployment{
		Deployment: daddr.ID(),
		Version:    hash,
	})
	if err != nil {
		session.Log().Error("error sending tx", "error", err)
		return err
	}

	return doSendManifest(session, signer, daddr.ID(), mani)
}

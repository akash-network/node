package deployment

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider/http"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func sendManifestCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "sendmani <manifest> <deployment>",
		Short: "send manifest to all deployment providers",
		Args:  cobra.ExactArgs(2),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(sendManifest))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())

	return cmd
}

func sendManifest(session session.Session, cmd *cobra.Command, args []string) error {
	signer, _, err := session.Signer()
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

	depAddr, err := keys.ParseDeploymentPath(args[1])
	if err != nil {
		return err
	}

	if err := manifestValidateResources(session, mani, depAddr); err != nil {
		return err
	}

	return doSendManifest(session, signer, depAddr.ID(), mani)
}
func doSendManifest(session session.Session, signer txutil.Signer, daddr []byte, mani *types.Manifest) error {
	leases, err := session.QueryClient().DeploymentLeases(session.Ctx(), daddr)
	if err != nil {
		return err
	}

	for _, lease := range leases.Items {
		if lease.State != types.Lease_ACTIVE {
			continue
		}
		provider, err := session.QueryClient().Provider(session.Ctx(), lease.Provider)
		if err != nil {
			return err
		}
		err = http.SendManifest(session.Ctx(), mani, signer, provider, lease.Deployment)
		if err != nil {
			return err
		}
	}
	return nil
}

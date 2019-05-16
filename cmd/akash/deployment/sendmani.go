package deployment

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common/sdutil"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider/http"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/dsky"
	"github.com/spf13/cobra"
)

func sendManifestCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "sendmani <config> <deployment>",
		Short: "send manifest to all deployment providers",
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
	var argPath, argAddr string
	if len(args) > 0 {
		argPath = args[0]
	}
	argPath = session.Mode().Ask().StringVar(argPath, "Deployment File Path (required): ", true)
	if len(args) > 1 {
		argAddr = args[1]
	}
	argAddr = session.Mode().Ask().StringVar(argAddr, "Deployment ID (required): ", true)

	// read the manifest from sdl
	sdl, err := sdl.ReadFile(argPath)
	if err != nil {
		return err
	}
	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	depAddr, err := keys.ParseDeploymentPath(argAddr)
	if err != nil {
		return err
	}

	if err := manifestValidateResources(session, mani, depAddr); err != nil {
		return err
	}
	log := session.Mode().Printer().Log().WithModule("deploy")
	msg := fmt.Sprintf("upload manifest for deployment (%s)", argAddr)
	log.WithAction(dsky.LogActionWait).Warn(msg)

	return doSendManifest(session, signer, depAddr.ID(), mani)
}

func doSendManifest(session session.Session, signer txutil.Signer, daddr []byte, mani *types.Manifest) error {
	log := session.Mode().Printer().Log().WithModule("deploy.sendmani")
	leases, err := session.QueryClient().DeploymentLeases(session.Ctx(), daddr)
	if err != nil {
		return err
	}
	raw := make([]interface{}, 0)
	raw = append(raw, leases)
	data := session.Mode().Printer().NewSection("Lease").WithLabel("Lease(s)").NewData()
	for _, lease := range leases.Items {
		sdutil.AppendLease(lease, data)
		if lease.State != types.Lease_ACTIVE {
			continue
		}
		provider, err := session.QueryClient().Provider(session.Ctx(), lease.Provider)
		if err != nil {
			return err
		}

		pd := dsky.NewSectionData("")
		sdutil.AppendProvider(provider, pd)
		msg := fmt.Sprintf("upload manifest to provider (%s)", X(provider.Address))
		log.WithAction(dsky.LogActionWait).Warn(msg)
		err = http.SendManifest(session.Ctx(), mani, signer, provider, lease.Deployment)
		if err != nil {
			return err
		}
		msg = fmt.Sprintf("manifest received by provider (%s)", X(provider.Address))
		pd.Add("Received Manifest", "Yes")
		data.Add("Provider", pd)
		raw = append(raw, provider)
		data.WithTag("raw", raw)
		log.WithAction(dsky.LogActionDone).Info(msg)
	}
	return session.Mode().Printer().Flush()
}

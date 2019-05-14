package deployment

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider/http"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/dsky"
	"github.com/spf13/cobra"
)

func statusDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "status <deployment-id>",
		Short: "get deployment status",
		RunE:  session.WithSession(session.RequireNode(statusDeployment)),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	return cmd
}

func statusDeployment(session session.Session, cmd *cobra.Command, args []string) error {
	var id string
	if len(args) > 0 {
		id = args[0]
	}
	id = session.Mode().Ask().StringVar(id, "Deployment ID (required): ", true)
	if len(id) == 0 {
		return errors.NewArgumentError("deployment:id")
	}

	deployment, err := keys.ParseDeploymentPath(id)
	if err != nil {
		return err
	}

	leases, err := session.QueryClient().DeploymentLeases(session.Ctx(), deployment.ID())
	if err != nil {
		return err
	}

	ld := session.Mode().Printer().NewSection("Lease").WithLabel("Lease(s)").NewData()
	var exitErr error
	for _, lease := range leases.Items {
		AppendLeaseSD(lease, ld)
		if lease.State != types.Lease_ACTIVE {
			continue
		}
		provider, err := session.QueryClient().Provider(session.Ctx(), lease.Provider)
		if err != nil {
			session.Log().Error("error fetching provider", "err", err, "lease", lease.LeaseID)
			exitErr = err
			continue
		}
		status, err := http.LeaseStatus(session.Ctx(), provider, lease.LeaseID)
		if err != nil {
			session.Log().Error("error fetching status ", "err", err, "lease", lease.LeaseID)
			exitErr = err
			continue
		}

		sd := dsky.NewSectionData("").AsList()
		for _, service := range status.Services {
			sd.Add("Name", service.Name)
			for _, uri := range service.URIs {
				sd.Add("Hosts", uri).WithLabel("Hosts", "Host(s) / IP(s)")
			}
			sd.Add("Available", service.Available)
			sd.Add("Total", service.Total)
		}
		ld.Add("Services", sd).WithLabel("Services", "Service(s)")

	}
	// data := session.Mode().Printer().NewSection("Deployment Status").NewData()
	// data.Add("Deployment ID", id)
	if err := session.Mode().Printer().Flush(); err != nil {
		return err
	}
	return exitErr
}

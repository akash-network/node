package deployment

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/validation"
	"github.com/spf13/cobra"
)

func validateDeploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <deployment-file>",
		Short: "validate deployment file",
		RunE:  session.WithSession(doValidateDeploymentCommand),
	}

	return cmd
}

func doValidateDeploymentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	var argPath string
	if len(args) > 0 {
		argPath = args[0]
	}
	argPath = session.Mode().Ask().StringVar(argPath, "Deployment File Path (required): ", true)
	_, err := sdl.ReadFile(argPath)
	if err != nil {
		return err
	}
	session.Mode().Printer().NewSection("Validate Deployment Config").NewData().
		Add("Result", "Valid")
	return session.Mode().Printer().Flush()
}

func manifestValidateResources(session session.Session, mani *types.Manifest, daddr []byte) error {
	dgroups, err := session.QueryClient().DeploymentGroupsForDeployment(session.Ctx(), daddr)
	if err != nil {
		return err
	}
	return validation.ValidateManifestWithDeployment(mani, dgroups.Items)
}

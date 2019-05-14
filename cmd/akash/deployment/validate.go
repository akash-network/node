package deployment

import (
	"fmt"

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
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(doValidateDeploymentCommand),
	}

	return cmd
}

func doValidateDeploymentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	_, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func manifestValidateResources(session session.Session, mani *types.Manifest, daddr []byte) error {
	dgroups, err := session.QueryClient().DeploymentGroupsForDeployment(session.Ctx(), daddr)
	if err != nil {
		return err
	}
	return validation.ValidateManifestWithDeployment(mani, dgroups.Items)
}

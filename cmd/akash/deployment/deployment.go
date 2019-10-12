package deployment

import "github.com/spf13/cobra"

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Manage deployments",
	}
	cmd.AddCommand(CreateCmd())
	cmd.AddCommand(CloseCmd())
	cmd.AddCommand(statusDeploymentCommand())
	cmd.AddCommand(updateDeploymentCommand())
	cmd.AddCommand(sendManifestCommand())
	cmd.AddCommand(validateDeploymentCommand())
	return cmd
}

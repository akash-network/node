package query

import (
	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func QueryCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "query [something]",
		Short: "query something",
		Args:  cobra.ExactArgs(1),
		// RunE:  withContext(requireNode(doQueryCommand)),
	}

	cmd.Flags().StringP(constants.FlagNode, "n", constants.DefaultNode, "node host")
	viper.BindPFlag(constants.FlagNode, cmd.Flags().Lookup(constants.FlagNode))

	cmd.AddCommand(queryAccountCommand(), queryDeploymentCommand())

	return cmd
}

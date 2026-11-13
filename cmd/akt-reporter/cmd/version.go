package cmd

import (
	"fmt"
	"runtime"

	"github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the service name, version, commit, platform and Go version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "service %s\nversion %s\ncommit %s\nplatform %s_%s\ngo %s\n",
				serviceName, version.Version, version.Commit, runtime.GOOS, runtime.GOARCH, runtime.Version())

			return err
		},
	}
}

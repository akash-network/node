package cmd

import (
	"fmt"
	"io"
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
			return writeVersion(cmd.OutOrStdout())
		},
	}
}

func writeVersion(out io.Writer) error {
	_, err := fmt.Fprintf(out, "service %s\nversion %s\ncommit %s\nplatform %s_%s\ngo %s\n",
		serviceName, version.Version, version.Commit, runtime.GOOS, runtime.GOARCH, runtime.Version())

	return err
}

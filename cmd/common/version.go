package common

import (
	"fmt"

	"github.com/ovrclk/akash/version"
	"github.com/spf13/cobra"
)

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("version: ", version.Version())
			fmt.Println("commit:  ", version.Commit())
			fmt.Println("date:    ", version.Date())
		},
	}
}

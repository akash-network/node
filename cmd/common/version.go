package common

import (
	"fmt"

	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/version"
	"github.com/spf13/cobra"
)

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Run: func(_ *cobra.Command, _ []string) {
			t := uitable.New().
				AddRow("Version:", version.Version()).
				AddRow("Commit:", version.Commit()).
				AddRow("Date:", version.Date())
			fmt.Println(t.String())
		},
	}
}

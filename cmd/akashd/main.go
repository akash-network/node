package main

import (
	"os"

	"github.com/ovrclk/akash/cmd/common"
)

func main() {
	root := baseCommand()
	root.AddCommand(initCommand())
	root.AddCommand(startCommand())
	root.AddCommand(common.VersionCommand())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

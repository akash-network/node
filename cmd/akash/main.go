package main

import (
	"os"

	_ "pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/cmd/akash/cmd"
)

// In main we call the rootCmd
func main() {
	rootCmd, _ := cmd.NewRootCmd()

	if err := cmd.Execute(rootCmd, "AKASH"); err != nil {
		os.Exit(1)
	}
}

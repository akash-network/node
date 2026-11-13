// Command akt-reporter is the Akash sink reporter.
package main

import (
	"os"

	"pkg.akt.dev/node/v2/cmd/akt-reporter/cmd"
)

func main() {
	os.Exit(cmd.Execute(cmd.NewRootCmd()))
}

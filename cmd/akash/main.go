package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/ovrclk/akash/cmd/akash/query"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	root := baseCommand()
	root.AddCommand(keyCommand())
	root.AddCommand(sendCommand())
	root.AddCommand(deploymentCommand())
	root.AddCommand(providerCommand())
	root.AddCommand(query.QueryCommand())
	root.AddCommand(statusCommand())
	root.AddCommand(marketplaceCommand())
	root.AddCommand(logsCommand())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

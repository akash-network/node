package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/ovrclk/akash/cmd/akash/deployment"
	"github.com/ovrclk/akash/cmd/akash/query"
	"github.com/ovrclk/akash/cmd/common"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	root := baseCommand()
	root.AddCommand(keyCommand())
	root.AddCommand(sendCommand())
	root.AddCommand(deploymentCommand())
	root.AddCommand(deployment.Command())
	root.AddCommand(providerCommand())
	root.AddCommand(query.QueryCommand())
	root.AddCommand(statusCommand())
	root.AddCommand(marketplaceCommand())
	root.AddCommand(logsCommand())
	root.AddCommand(common.VersionCommand())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

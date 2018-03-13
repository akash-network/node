package main

import (
	"math/rand"
	"time"

	"github.com/ovrclk/photon/cmd/photon/query"
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
	root.Execute()
}

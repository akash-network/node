package main

import "github.com/ovrclk/photon/cmd/photon/query"

func main() {
	root := baseCommand()
	root.AddCommand(keyCommand())
	root.AddCommand(sendCommand())
	root.AddCommand(deploymentCommand())
	root.AddCommand(datacenterCommand())
	root.AddCommand(query.QueryCommand())
	root.AddCommand(pingCommand())
	root.AddCommand(marketplaceCommand())
	root.Execute()
}

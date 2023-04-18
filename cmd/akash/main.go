package main

import (
	"os"

	"github.com/cosmos/cosmos-sdk/server"

	_ "github.com/akash-network/akash-api/go/sdkutil"

	"github.com/akash-network/node/cmd/akash/cmd"
)

// In main we call the rootCmd
func main() {
	rootCmd, _ := cmd.NewRootCmd()

	if err := cmd.Execute(rootCmd, "AKASH"); err != nil {
		switch e := err.(type) {
		case server.ErrorCode:
			os.Exit(e.Code)
		default:
			os.Exit(1)
		}
	}
}

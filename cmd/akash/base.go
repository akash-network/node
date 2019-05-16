package main

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/spf13/cobra"
)

func baseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "akash",
		Short:         "Akash CLI Utility",
		Long:          baseLongDesc,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	session.SetupBaseCommand(cmd)
	return cmd
}

var baseLongDesc = `Akash CLI Utility. 

Akash is a peer-to-peer marketplace for computing resources and 
a deployment platform for heavily distributed applications. 
Find out more at https://akash.network`

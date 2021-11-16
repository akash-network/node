package cmd

import (
	"fmt"

	cutil "github.com/ovrclk/akash/provider/cluster/util"
	mcli "github.com/ovrclk/akash/x/market/client/cli"
	"github.com/spf13/cobra"
)

func clusterNSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "show-cluster-ns",
		Aliases:      []string{"cluster-ns"},
		Short:        "print cluster namespace for given lease ID",
		Args:         cobra.ExactArgs(0),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			lid, err := mcli.LeaseIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}
			fmt.Println(cutil.LeaseIDToNamespace(lid))
			return nil
		},
	}
	mcli.AddLeaseIDFlags(cmd.Flags())
	mcli.MarkReqLeaseIDFlags(cmd)
	return cmd
}

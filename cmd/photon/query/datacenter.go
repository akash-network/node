package query

import (
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
)

func queryDatacenterCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "datacenter",
		Short: "query datacenter",
		RunE:  context.WithContext(context.RequireNode(doQueryDatacenterCommand)),
	}

	return cmd
}

func doQueryDatacenterCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.DatacenterPath
	if len(args) > 0 {
		structure := new(types.Datacenter)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.Datacenters)
		return doQuery(ctx, path, structure)
	}
}

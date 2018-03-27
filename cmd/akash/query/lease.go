package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func queryLeaseCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "lease [deployment]",
		Short: "query lease",
		RunE:  context.WithContext(context.RequireNode(doQueryLeaseCommand)),
	}

	return cmd
}

func doQueryLeaseCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.LeasePath
	if len(args) > 0 {
		structure := new(types.Lease)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.Leases)
		return doQuery(ctx, path, structure)
	}
}

package query

import (
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
)

func queryOrderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "order",
		Short: "query order",
		RunE:  context.WithContext(context.RequireNode(doQueryOrderCommand)),
	}

	return cmd
}

func doQueryOrderCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.OrderPath
	if len(args) > 0 {
		structure := new(types.Order)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.Orders)
		return doQuery(ctx, path, structure)
	}
}

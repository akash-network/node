package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func queryProviderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "provider",
		Short: "query provider",
		RunE:  context.WithContext(context.RequireNode(doQueryProviderCommand)),
	}

	return cmd
}

func doQueryProviderCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	path := state.ProviderPath
	if len(args) > 0 {
		structure := new(types.Provider)
		path += args[0]
		return doQuery(ctx, path, structure)
	} else {
		structure := new(types.Providers)
		return doQuery(ctx, path, structure)
	}
}

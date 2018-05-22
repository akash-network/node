package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/keys"
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
	if len(args) == 0 {
		handleMessage(ctx.QueryClient().Leases(ctx.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseLeasePath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(ctx.QueryClient().Lease(ctx.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

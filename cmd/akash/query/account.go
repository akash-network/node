package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryAccountCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "account",
		Short: "query account",
		Args:  cobra.ExactArgs(1),
		RunE:  context.WithContext(context.RequireNode(doQueryAccountCommand)),
	}

	return cmd
}

func doQueryAccountCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		key, err := keys.ParseAccountPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(ctx.QueryClient().Account(ctx.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

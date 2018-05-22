package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/keys"
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
	if len(args) == 0 {
		handleMessage(ctx.QueryClient().Orders(ctx.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseOrderPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(ctx.QueryClient().Order(ctx.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

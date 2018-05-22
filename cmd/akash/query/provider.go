package query

import (
	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/keys"
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
	if len(args) == 0 {
		handleMessage(ctx.QueryClient().Providers(ctx.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseProviderPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(ctx.QueryClient().Provider(ctx.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

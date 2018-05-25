package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryProviderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "provider",
		Short: "query provider",
		RunE:  session.WithSession(session.RequireNode(doQueryProviderCommand)),
	}

	return cmd
}

func doQueryProviderCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		handleMessage(session.QueryClient().Providers(session.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseProviderPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Provider(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

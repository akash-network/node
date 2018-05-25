package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryAccountCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "account",
		Short: "query account",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(doQueryAccountCommand)),
	}

	return cmd
}

func doQueryAccountCommand(session session.Session, cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		key, err := keys.ParseAccountPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Account(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryOrderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "order [order ...]",
		Short: "query order",
		RunE:  session.WithSession(session.RequireNode(doQueryOrderCommand)),
	}

	return cmd
}

func doQueryOrderCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return handleMessage(session.QueryClient().Orders(session.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseOrderPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Order(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryFulfillmentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "fulfillment [fulfillment ...]",
		Short: "query fulfillment",
		RunE:  session.WithSession(session.RequireNode(doQueryFulfillmentCommand)),
	}

	return cmd
}

func doQueryFulfillmentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return handleMessage(session.QueryClient().Fulfillments(session.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseFulfillmentPath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Fulfillment(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

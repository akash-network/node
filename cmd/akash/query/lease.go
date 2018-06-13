package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryLeaseCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "lease [lease ...]",
		Short: "query lease",
		RunE:  session.WithSession(session.RequireNode(doQueryLeaseCommand)),
	}

	return cmd
}

func doQueryLeaseCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		handleMessage(session.QueryClient().Leases(session.Ctx()))
	}
	for _, arg := range args {
		key, err := keys.ParseLeasePath(arg)
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Lease(session.Ctx(), key.ID())); err != nil {
			return err
		}
	}
	return nil
}

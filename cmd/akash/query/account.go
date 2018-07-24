package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/spf13/cobra"
)

func queryAccountCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "account [account ...]",
		Short: "query account",
		RunE:  session.WithSession(session.RequireNode(doQueryAccountCommand)),
	}
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	return cmd
}

func doQueryAccountCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		_, info, err := session.Signer()
		if err != nil {
			return err
		}
		if err := handleMessage(session.QueryClient().Account(session.Ctx(), info.PubKey.Address().Bytes())); err != nil {
			return err
		}
		return nil
	}
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

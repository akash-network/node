package query

import (
	"fmt"

	humanize "github.com/dustin/go-humanize"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func queryLeaseCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "lease [lease ...]",
		Short: "query lease",
		RunE:  session.WithSession(session.RequireNode(doQueryLeaseCommand)),
	}
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	return cmd
}

func doQueryLeaseCommand(s session.Session, cmd *cobra.Command, args []string) error {
	leases := make([]*types.Lease, 0)
	var hasSigner, hasIDs bool
	hasIDs = len(args) > 0
	_, info, err := s.Signer()
	if err == nil {
		hasSigner = true
	}

	switch {
	case hasSigner == false && hasIDs == false:
		var id string
		id = s.Mode().Ask().StringVar(id, "Lease ID (required): ", true)
		if len(id) == 0 {
			return fmt.Errorf("required argument missing: id")
		}
		args = []string{id}
		hasIDs = true
		fallthrough
	case hasIDs:
		for _, arg := range args {
			key, err := keys.ParseLeasePath(arg)
			if err != nil {
				return err
			}
			lease, err := s.QueryClient().Lease(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			leases = append(leases, lease)
		}
	case hasSigner:
		res, err := s.QueryClient().TenantLeases(s.Ctx(), info.GetPubKey().Address().Bytes())
		if err != nil {
			return err
		}
		leases = res.Items
	}

	data := s.Mode().Printer().NewSection("Lease(s)").NewData().WithTag("raw", leases)
	if len(leases) > 1 {
		data.AsList()
	}
	for _, l := range leases {
		data.
			Add("Lease", l.LeaseID.String()).
			Add("Price", humanize.Comma(int64(l.Price))).
			Add("State", l.State.String())
	}
	return s.Mode().Printer().Flush()
}

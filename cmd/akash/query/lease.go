package query

import (
	"fmt"

	humanize "github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/uiutil"
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
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)

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

	for _, l := range leases {
		printerDat.AddResultList(makePrinterResultLease(l))
		rawDat = append(rawDat, l)
	}
	printerDat.Raw = rawDat

	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			t := uitable.New().AddRow(
				uiutil.NewTitle("Lease ID (Deployment/Group/Order/Provider)").String(),
				uiutil.NewTitle("Price").String(),
				uiutil.NewTitle("State").String(),
			)
			t.Wrap = true
			for _, l := range leases {
				res := makePrinterResultLease(l)
				t.AddRow(res["lease"], res["price"], res["state"])
			}
			return session.NewIPrinter(nil).AddText("").Add(t).Flush()
		}).
		When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

func makePrinterResultLease(lease *types.Lease) session.PrinterResult {
	return session.PrinterResult{
		"lease": lease.LeaseID.String(),
		"price": humanize.Comma(int64(lease.Price)),
		"state": lease.State.String(),
	}
}

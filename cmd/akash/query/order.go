package query

import (
	"strconv"

	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/uiutil"
	"github.com/ovrclk/dsky"
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

func doQueryOrderCommand(s session.Session, cmd *cobra.Command, args []string) error {
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)
	orders := make([]*types.Order, 0)

	if len(args) > 0 {
		for _, arg := range args {
			key, err := keys.ParseOrderPath(arg)
			if err != nil {
				return err
			}
			order, err := s.QueryClient().Order(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			orders = append(orders, order)
		}
	} else {
		res, err := s.QueryClient().Orders(s.Ctx())
		if err != nil {
			return err
		}
		orders = res.Items
	}

	for _, order := range orders {
		printerDat.AddResultList(makePrinterResultOrder(order))
		rawDat = append(rawDat, order)
	}
	printerDat.Raw = rawDat

	return s.Mode().
		When(dsky.ModeTypeInteractive, func() error {
			t := uitable.New().AddRow(
				uiutil.NewTitle("Order ID (Deployment/Group/Sequence)").String(),
				uiutil.NewTitle("End At (Block)").String(),
				uiutil.NewTitle("State").String(),
			)
			t.Wrap = true
			for _, o := range orders {
				res := makePrinterResultOrder(o)
				t.AddRow(res["order"], res["end_at"], res["state"])
			}
			return session.NewIPrinter(nil).AddText("").Add(t).Flush()
		}).
		When(dsky.ModeTypeShell, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(dsky.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

func makePrinterResultOrder(order *types.Order) session.PrinterResult {
	return session.PrinterResult{
		"order":  order.OrderID.String(),
		"end_at": strconv.FormatInt(order.EndAt, 10),
		"state":  order.State.String(),
	}
}

package query

import (
	humanize "github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/uiutil"
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

func doQueryFulfillmentCommand(s session.Session, cmd *cobra.Command, args []string) error {
	fulfillments := make([]*types.Fulfillment, 0)
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)

	if len(args) == 0 {
		res, err := s.QueryClient().Fulfillments(s.Ctx())
		if err != nil {
			return err
		}
		fulfillments = res.Items
	} else {
		for _, arg := range args {
			key, err := keys.ParseFulfillmentPath(arg)
			if err != nil {
				return err
			}
			fulfillment, err := s.QueryClient().Fulfillment(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			fulfillments = append(fulfillments, fulfillment)
		}
	}

	for _, f := range fulfillments {
		printerDat.AddResultList(makePrinterResultFulfillment(f))
		rawDat = append(rawDat, f)
	}
	printerDat.Raw = rawDat

	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			t := uitable.New().AddRow(
				uiutil.NewTitle("Fulfillment ID (Deployment/Group/Order/Provider)").String(),
				uiutil.NewTitle("Price").String(),
				uiutil.NewTitle("State").String(),
			)
			t.Wrap = true
			for _, f := range fulfillments {
				res := makePrinterResultFulfillment(f)
				t.AddRow(res["fulfillment"], res["price"], res["state"])
			}
			return session.NewIPrinter(nil).AddText("").Add(t).Flush()
		}).
		When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

func makePrinterResultFulfillment(f *types.Fulfillment) session.PrinterResult {
	return session.PrinterResult{
		"fulfillment": f.FulfillmentID.String(),
		"price":       humanize.Comma(int64(f.Price)),
		"state":       f.State.String(),
	}
}

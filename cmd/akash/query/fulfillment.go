package query

import (
	humanize "github.com/dustin/go-humanize"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/dsky"
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

	data := s.Mode().Printer().NewSection("Fulfillment(s)").NewData().WithTag("raw", fulfillments).AsList()
	for _, f := range fulfillments {
		data.
			Add("Fulfillment ID", f.FulfillmentID.String()).
			Add("Price", humanize.Comma(int64(f.Price)))

		switch f.State {
		case types.Fulfillment_OPEN:
			data.Add("State", dsky.Color.Hi.Sprint(f.State.String()))
		case types.Fulfillment_MATCHED:
			data.Add("State", dsky.Color.Notice.Sprint(f.State.String()))
		default:
			data.Add("State", f.State.String())
		}
	}
	return s.Mode().Printer().Flush()
}

func makePrinterResultFulfillment(f *types.Fulfillment) session.PrinterResult {
	return session.PrinterResult{
		"fulfillment": f.FulfillmentID.String(),
		"price":       humanize.Comma(int64(f.Price)),
		"state":       f.State.String(),
	}
}

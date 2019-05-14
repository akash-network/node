package query

import (
	"strconv"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
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

	data := s.Mode().Printer().NewSection("Orders(s)").NewData().WithTag("raw", orders)
	if len(orders) > 1 {
		data.AsList()
	}
	for _, order := range orders {
		data.
			Add("Order", order.OrderID.String()).
			Add("End At", strconv.FormatInt(order.EndAt, 10)).
			WithLabel("End At", "End At (Block)").
			Add("State", order.State.String())
	}
	return s.Mode().Printer().Flush()
}

package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"

	"pkg.akt.dev/go/node/market/v1beta5"
)

// GetQueryCmd returns the transaction commands for the market module
func GetQueryCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:                        v1beta5.ModuleName,
		Short:                      "Market query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		getOrderCmd(),
		getBidCmd(),
		getLeaseCmd(),
	)

	return cmd
}

func getOrderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "order",
		Short:                      "Order query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetOrders(),
		cmdGetOrder(),
	)

	return cmd
}

func getBidCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "bid",
		Short:                      "Bid query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetBids(),
		cmdGetBid(),
	)

	return cmd
}

func getLeaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "lease",
		Short:                      "Lease query commands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetLeases(),
		cmdGetLease(),
	)

	return cmd
}

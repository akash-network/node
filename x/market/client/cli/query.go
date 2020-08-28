package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

// GetQueryCmd returns the transaction commands for the market module
func GetQueryCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Market query commands",
		DisableFlagParsing:         true,
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
		DisableFlagParsing:         true,
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
		DisableFlagParsing:         true,
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
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		cmdGetLeases(),
		cmdGetLease(),
	)

	return cmd
}

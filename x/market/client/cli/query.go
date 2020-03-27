package cli

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

// GetQueryCmd returns the transaction commands for the market module
func GetQueryCmd(key string, cdc *codec.Codec) *cobra.Command {

	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Market query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		getOrderCmd(key, cdc),
		getBidCmd(key, cdc),
		getLeaseCmd(key, cdc),
	)...)

	return cmd
}

func getOrderCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "order",
		Short:                      "Order query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdGetOrders(key, cdc),
	)...)

	return cmd
}

func getBidCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "bid",
		Short:                      "Bid query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdGetBids(key, cdc),
	)...)

	return cmd
}

func getLeaseCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "lease",
		Short:                      "Lease query commands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(flags.GetCommands(
		cmdGetLeases(key, cdc),
	)...)

	return cmd
}

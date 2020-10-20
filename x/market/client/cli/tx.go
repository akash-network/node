package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

// GetTxCmd returns the transaction commands for market module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		cmdCreateBid(key),
		cmdCloseBid(key),
		cmdCloseOrder(key),
	)
	return cmd
}

func cmdCreateBid(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid-create",
		Short: fmt.Sprintf("Create a %s bid", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadTxCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			price, err := cmd.Flags().GetString("price")
			if err != nil {
				return err
			}

			coins, err := sdk.ParseCoin(price)
			if err != nil {
				return err
			}

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCreateBid{
				Order:    id,
				Provider: clientCtx.GetFromAddress().String(),
				Price:    coins,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddOrderIDFlags(cmd.Flags())
	cmd.Flags().String("price", "", "Bid Price")

	return cmd
}

func cmdCloseBid(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid-close",
		Short: fmt.Sprintf("Close a %s bid", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadTxCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			id, err := BidIDFromFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCloseBid{
				BidID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddBidIDFlags(cmd.Flags())

	return cmd
}

func cmdCloseOrder(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "order-close",
		Short: fmt.Sprintf("Close a %s order", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			clientCtx, err := client.ReadTxCommandFlags(clientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCloseOrder{
				OrderID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddOrderIDFlags(cmd.Flags())

	return cmd
}

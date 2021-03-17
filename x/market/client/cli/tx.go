package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

// GetTxCmd returns the transaction commands for market module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		cmdBid(key),
		cmdLease(key),
	)
	return cmd
}

func cmdBid(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "bid",
		Short:                      "Bid subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		cmdBidCreate(key),
		cmdBidClose(key),
	)
	return cmd
}

func cmdBidCreate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: fmt.Sprintf("Create a %s bid", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			price, err := cmd.Flags().GetString("price")
			if err != nil {
				return err
			}

			coin, err := sdk.ParseDecCoin(price)
			if err != nil {
				return err
			}

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			deposit, err := common.DepositFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCreateBid{
				Order:    id,
				Provider: clientCtx.GetFromAddress().String(),
				Price:    coin,
				Deposit:  deposit,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddOrderIDFlags(cmd.Flags())
	cmd.Flags().String("price", "", "Bid Price")
	common.AddDepositFlags(cmd.Flags(), DefaultDeposit)

	return cmd
}

func cmdBidClose(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close a %s bid", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
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

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddBidIDFlags(cmd.Flags())

	return cmd
}

func cmdLease(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "lease",
		Short:                      "Lease subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(
		cmdLeaseCreate(key),
		cmdLeaseWithdraw(key),
		cmdLeaseClose(key),
	)
	return cmd
}

func cmdLeaseCreate(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: fmt.Sprintf("Create a %s lease", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := BidIDFromFlagsWithoutCtx(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCreateLease{
				BidID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddLeaseIDFlags(cmd.Flags())
	MarkReqLeaseIDFlags(cmd)

	return cmd
}

func cmdLeaseWithdraw(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "withdraw",
		Short: fmt.Sprintf("Settle and withdraw available funds from %s order escrow account", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := LeaseIDFromFlagsWithoutCtx(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgWithdrawLease{
				LeaseID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddLeaseIDFlags(cmd.Flags())
	MarkReqLeaseIDFlags(cmd)

	return cmd
}

func cmdLeaseClose(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close a %s order", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			id, err := LeaseIDFromFlagsWithoutCtx(cmd.Flags())
			if err != nil {
				return err
			}

			msg := &types.MsgCloseLease{
				LeaseID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return sdkutil.BroadcastTX(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	AddLeaseIDFlags(cmd.Flags())
	MarkReqLeaseIDFlags(cmd)

	return cmd
}

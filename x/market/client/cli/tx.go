package cli

import (
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/node/market/v1beta5"

	cltypes "pkg.akt.dev/go/node/client/types"

	aclient "pkg.akt.dev/akashd/client"
	"pkg.akt.dev/akashd/cmd/common"
)

// GetTxCmd returns the transaction commands for market module
func GetTxCmd(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        v1beta5.ModuleName,
		Short:                      "Transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
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
		RunE:                       sdkclient.ValidateCmd,
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
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			opts, err := cltypes.ClientOptionsFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, opts...)
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

			id, err := cli.OrderIDFromFlags(cmd.Flags(), cli.WithProvider(cctx.FromAddress))
			if err != nil {
				return err
			}

			deposit, err := common.DetectDeposit(ctx, cmd.Flags(), cl.Query(), "market", "BidMinDeposit")
			if err != nil {
				return err
			}

			msg := &v1beta5.MsgCreateBid{
				OrderID:  id,
				Provider: cctx.GetFromAddress().String(),
				Price:    coin,
				Deposit:  deposit,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	cli.AddTxFlagsToCmd(cmd)
	cli.AddOrderIDFlags(cmd.Flags())
	cmd.Flags().String("price", "", "Bid Price")
	common.AddDepositFlags(cmd.Flags())

	return cmd
}

func cmdBidClose(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close a %s bid", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			opts, err := cltypes.ClientOptionsFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, opts...)
			if err != nil {
				return err
			}

			id, err := cli.BidIDFromFlags(cmd.Flags(), cli.WithProvider(cctx.FromAddress))
			if err != nil {
				return err
			}

			msg := &v1beta5.MsgCloseBid{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	cli.AddTxFlagsToCmd(cmd)
	cli.AddBidIDFlags(cmd.Flags())

	return cmd
}

func cmdLease(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "lease",
		Short:                      "Lease subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
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
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			opts, err := cltypes.ClientOptionsFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, opts...)
			if err != nil {
				return err
			}

			id, err := cli.LeaseIDFromFlags(cmd.Flags(), cli.WithOwner(cctx.FromAddress))
			if err != nil {
				return err
			}

			msg := &v1beta5.MsgCreateLease{
				BidID: id.BidID(),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	cli.AddTxFlagsToCmd(cmd)
	cli.AddLeaseIDFlags(cmd.Flags())
	cli.MarkReqLeaseIDFlags(cmd, cli.DeploymentIDOptionNoOwner(true))

	return cmd
}

func cmdLeaseWithdraw(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "withdraw",
		Short: fmt.Sprintf("Settle and withdraw available funds from %s order escrow account", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			opts, err := cltypes.ClientOptionsFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, opts...)
			if err != nil {
				return err
			}

			id, err := cli.LeaseIDFromFlags(cmd.Flags(), cli.WithOwner(cctx.FromAddress))
			if err != nil {
				return err
			}

			msg := &v1beta5.MsgWithdrawLease{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	cli.AddTxFlagsToCmd(cmd)
	cli.AddLeaseIDFlags(cmd.Flags())
	cli.MarkReqLeaseIDFlags(cmd, cli.DeploymentIDOptionNoOwner(true))

	return cmd
}

func cmdLeaseClose(key string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close",
		Short: fmt.Sprintf("Close a %s order", key),
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			opts, err := cltypes.ClientOptionsFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			cl, err := aclient.DiscoverClient(ctx, cctx, opts...)
			if err != nil {
				return err
			}

			id, err := cli.LeaseIDFromFlags(cmd.Flags(), cli.WithOwner(cctx.FromAddress))
			if err != nil {
				return err
			}

			msg := &v1beta5.MsgCloseLease{
				ID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			resp, err := cl.Tx().Broadcast(ctx, []sdk.Msg{msg})
			if err != nil {
				return err
			}

			return cl.PrintMessage(resp)
		},
	}

	cli.AddTxFlagsToCmd(cmd)
	cli.AddLeaseIDFlags(cmd.Flags())
	cli.MarkReqLeaseIDFlags(cmd, cli.DeploymentIDOptionNoOwner(true))

	return cmd
}

package cli

import (
	"os"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
)

// GetTxCmd returns the transaction commands for market module
func GetTxCmd(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Transaction subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	cmd.AddCommand(flags.PostCommands(
		cmdCreateBid(key, cdc),
		cmdCloseBid(key, cdc),
		cmdCloseOrder(key, cdc),
	)...)
	return cmd
}

func cmdCreateBid(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid-create",
		Short: "Create bid",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(authclient.GetTxEncoder(cdc))

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

			msg := types.MsgCreateBid{
				Order:    id,
				Provider: ctx.GetFromAddress(),
				Price:    coins,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return authclient.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddOrderIDFlags(cmd.Flags())
	cmd.Flags().String("price", "", "Bid Price")
	return cmd
}

func cmdCloseBid(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bid-close",
		Short: "Close bid",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(authclient.GetTxEncoder(cdc))

			id, err := BidIDFromFlags(ctx, cmd.Flags())
			if err != nil {
				return err
			}
			msg := types.MsgCloseBid{
				BidID: id,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return authclient.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddBidIDFlags(cmd.Flags())
	return cmd
}

func cmdCloseOrder(key string, cdc *codec.Codec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "order-close",
		Short: "Create bid",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.NewCLIContext().WithCodec(cdc)
			bldr := auth.NewTxBuilderFromCLI(os.Stdin).WithTxEncoder(authclient.GetTxEncoder(cdc))

			id, err := OrderIDFromFlags(cmd.Flags())
			if err != nil {
				return err
			}

			msg := types.MsgCloseOrder{
				OrderID: id,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return authclient.GenerateOrBroadcastMsgs(ctx, bldr, []sdk.Msg{msg})
		},
	}
	AddOrderIDFlags(cmd.Flags())
	return cmd
}

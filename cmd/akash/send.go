package main

import (
	"fmt"
	"strconv"

	"github.com/ovrclk/akash/cmd/akash/context"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/spf13/cobra"
)

func sendCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "send [amount] [to account]",
		Short: "send tokens",
		Args:  cobra.ExactArgs(2),
		RunE: context.WithContext(
			context.RequireKey(context.RequireNode(doSendCommand))),
	}

	context.AddFlagNode(cmd, cmd.Flags())
	context.AddFlagKey(cmd, cmd.Flags())
	context.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func doSendCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	txclient, err := ctx.TxClient()
	if err != nil {
		return err
	}

	amount, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return err
	}

	to, err := base.DecodeString(args[1])
	if err != nil {
		return err
	}

	result, err := txclient.BroadcastTxCommit(&types.TxSend{
		From:   txclient.Key().Address(),
		To:     to,
		Amount: amount,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Sent %v tokens to %v in block %v\n", amount, to.EncodeString(), result.Height)

	return nil
}

package main

import (
	"fmt"
	"strconv"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func sendCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "send [amount] [to account]",
		Short: "send tokens",
		Args:  cobra.ExactArgs(2),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(doSendCommand))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func doSendCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	amount, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return err
	}

	to, err := keys.ParseAccountPath(args[1])
	if err != nil {
		return err
	}

	result, err := txclient.BroadcastTxCommit(&types.TxSend{
		From:   txclient.Key().Address(),
		To:     to.ID(),
		Amount: amount,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Sent %v tokens to %v in block %v\n", amount, to.ID(), result.Height)

	return nil
}

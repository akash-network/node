package main

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/denom"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/spf13/cobra"
)

func sendCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "send [amount] [to account]",
		Short: "Send tokens",
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

	amount, err := denom.ToBase(args[0])
	if err != nil {
		return err
	}

	to, err := keys.ParseAccountPath(args[1])
	if err != nil {
		return err
	}

	result, err := txclient.BroadcastTxCommit(&types.TxSend{
		From:   txclient.Key().GetPubKey().Address().Bytes(),
		To:     to.ID(),
		Amount: amount,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Sent %v tokens to %s in block %v\n", args[0], to.ID(), result.Height)

	return nil
}

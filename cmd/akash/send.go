package main

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/denom"
	"github.com/ovrclk/akash/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
)

func sendCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "send [amount] [receiver]",
		Short: "Send tokens",
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(doSendCommand))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	return cmd
}

func doSendCommand(s session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := s.TxClient()
	if err != nil {
		return err
	}

	var argAmount, argTo string
	if len(args) == 2 {
		argAmount = args[0]
		argTo = args[1]
	}

	argAmount = s.Mode().Ask().StringVar(argAmount, "Amount (required): ", true)
	if len(argAmount) == 0 {
		return errors.NewArgumentError("amount")
	}

	argTo = s.Mode().Ask().StringVar(argTo, "Receiver Address (required): ", true)
	if len(argTo) == 0 {
		return errors.NewArgumentError("receiver")
	}

	amount, err := denom.ToBase(argAmount)
	if err != nil {
		return err
	}

	to, err := keys.ParseAccountPath(argTo)
	if err != nil {
		return err
	}

	fromAddr := txclient.Key().GetPubKey().Address().Bytes()

	result, err := txclient.BroadcastTxCommit(&types.TxSend{
		From:   fromAddr,
		To:     to.ID(),
		Amount: amount,
	})
	if err != nil {
		return err
	}

	block := humanize.Comma(result.Height)
	amountF := humanize.Comma(int64(amount))
	s.Mode().Printer().Log().WithModule("tokens").Info(fmt.Sprintf("%d token(s) transfered to %v", amount, argTo))
	data := s.Mode().Printer().NewSection("Tokens").WithLabel("Send Token(s)").NewData().WithTag("raw", result)
	data.
		Add("From", X(fromAddr)).
		Add("To", argTo).
		Add("Amount", amountF).
		Add("Block", block).
		Add("Hash", X(result.Hash))
	return s.Mode().Printer().Flush()
}

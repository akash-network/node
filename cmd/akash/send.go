package main

import (
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/denom"
	"github.com/ovrclk/akash/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/util/ulog"
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

	block := result.Height

	printerDat := session.NewPrinterDataKV().
		AddResultKV("from", X(fromAddr)).
		AddResultKV("to", argTo).
		AddResultKV("amount", argAmount).
		AddResultKV("block", strconv.FormatInt(result.Height, 10)).
		AddResultKV("hash", X(result.Hash))

	printerDat.Raw = result

	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			res := printerDat.Result[0]
			t := uitable.New().
				AddRow("From:", res["from"]).
				AddRow("To:", res["to"]).
				AddRow("Amount:", res["amount"]).
				AddRow("Block (Height):", humanize.Comma(block)).
				AddRow("Hash:", res["hash"])

			p := session.NewIPrinter(nil).
				AddText("").
				AddTitle("Send Tokens").
				Add(t).
				AddText("")
			p.Flush()
			return p.AddText(ulog.Success("transfer complete")).Flush()
		}).
		When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

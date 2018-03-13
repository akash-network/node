package main

import (
	"fmt"
	"strconv"

	"github.com/ovrclk/photon/cmd/photon/constants"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/spf13/cobra"
	tmclient "github.com/tendermint/tendermint/rpc/client"
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
	kmgr, _ := ctx.KeyManager()
	key, _ := ctx.Key()

	amount, err := strconv.ParseUint(args[0], 10, 64)
	if err != nil {
		return err
	}

	to := new(base.Bytes)
	if err := to.DecodeString(args[1]); err != nil {
		return err
	}

	nonce, err := ctx.Nonce()
	if err != nil {
		return err
	}

	signer := txutil.NewKeystoreSigner(kmgr, key.Name, constants.Password)

	tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
		From:   base.Bytes(key.Address),
		To:     *to,
		Amount: amount,
	})
	if err != nil {
		return err
	}

	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	result, err := client.BroadcastTxCommit(tx)
	if err != nil {
		return err
	}
	if result.CheckTx.IsErr() {
		return errors.New(result.CheckTx.GetLog())
	}
	if result.DeliverTx.IsErr() {
		return errors.New(result.DeliverTx.GetLog())
	}

	fmt.Printf("Sent %v tokens to %v in block %v\n", amount, to.EncodeString(), result.Height)

	return nil
}

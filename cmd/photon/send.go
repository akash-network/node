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
	"github.com/spf13/viper"
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

	cmd.Flags().StringP(constants.FlagNode, "n", constants.DefaultNode, "node host")
	viper.BindPFlag(constants.FlagNode, cmd.Flags().Lookup(constants.FlagNode))

	cmd.Flags().StringP(constants.FlagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(constants.FlagKey)

	cmd.Flags().Uint64(constants.FlagNonce, 0, "nonce (optional)")

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

	tx, err := txutil.BuildTx(kmgr, key.Name, constants.Password, nonce, &types.TxSend{
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
	fmt.Println(result)

	return nil
}

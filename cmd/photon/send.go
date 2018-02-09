package main

import (
	"fmt"
	"strconv"

	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func sendCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "send [amount] [account]",
		Short: "send tokens",
		Args:  cobra.ExactArgs(2),
		RunE: withContext(
			requireKey(requireNode(doSendCommand))),
	}

	cmd.Flags().StringP(flagNode, "n", defaultNode, "node host")
	viper.BindPFlag(flagNode, cmd.Flags().Lookup(flagNode))

	cmd.Flags().StringP(flagKey, "k", "", "key name (required)")
	cmd.MarkFlagRequired(flagKey)

	return cmd
}

func doSendCommand(ctx Context, cmd *cobra.Command, args []string) error {
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

	tx, err := txutil.BuildTx(kmgr, key.Name, password, &types.TxSend{
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

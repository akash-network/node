package sdkutil

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	"github.com/spf13/pflag"
	ttypes "github.com/tendermint/tendermint/types"
)

const (
	broadcastBlockRetryTimeout = 300 * time.Second
	broadcastBlockRetryPeriod  = time.Second

	// sadface.

	// Only way to detect the timeout error.
	// https://github.com/tendermint/tendermint/blob/46e06c97320bc61c4d98d3018f59d47ec69863c9/rpc/core/mempool.go#L124
	timeoutErrorMessage = "timed out waiting for tx to be included in a block"

	// Only way to check for tx not found error.
	// https://github.com/tendermint/tendermint/blob/46e06c97320bc61c4d98d3018f59d47ec69863c9/rpc/core/tx.go#L31-L33
	notFoundErrorMessageSuffix = ") not found"
)

func BroadcastTX(ctx context.Context, cctx client.Context, flags *pflag.FlagSet, msgs ...sdk.Msg) error {

	// rewrite of https://github.com/cosmos/cosmos-sdk/blob/ca98fda6eae597b1e7d468f96d030b6d905748d7/client/tx/tx.go#L29
	// to add continuing retries if broadcast-mode=block fails with a timeout.

	txf := tx.NewFactoryCLI(cctx, flags)

	if cctx.GenerateOnly {
		return tx.GenerateTx(cctx, txf, msgs...)
	}

	txf, err := tx.PrepareFactory(cctx, txf)
	if err != nil {
		return err
	}

	txf, err = AdjustGas(cctx, txf, msgs...)
	if err != nil {
		return err
	}
	if cctx.Simulate {
		return nil
	}

	txb, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return err
	}

	ok, err := confirmTx(cctx, txb)
	if !ok || err != nil {
		return err
	}

	err = tx.Sign(txf, cctx.GetFromName(), txb, true)
	if err != nil {
		return err
	}

	txBytes, err := cctx.TxConfig.TxEncoder()(txb.GetTx())
	if err != nil {
		return err
	}

	res, err := doBroadcast(ctx, cctx, broadcastBlockRetryTimeout, txBytes)
	if err != nil {
		return err
	}

	return cctx.PrintProto(res)

}

func doBroadcast(ctx context.Context, cctx client.Context, timeout time.Duration, txb ttypes.Tx) (*sdk.TxResponse, error) {
	switch cctx.BroadcastMode {
	case flags.BroadcastSync:
		return cctx.BroadcastTxSync(txb)
	case flags.BroadcastAsync:
		return cctx.BroadcastTxAsync(txb)
	}

	hash := hex.EncodeToString(txb.Hash())

	// broadcast-mode=block
	// submit with mode commit/block
	cres, err := cctx.BroadcastTxCommit(txb)
	if err == nil {
		// good job
		return cres, nil
	} else if !strings.HasSuffix(err.Error(), timeoutErrorMessage) {
		return cres, err
	}

	// loop
	lctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for lctx.Err() == nil {

		// wait up to one second
		select {
		case <-lctx.Done():
			return cres, err
		case <-time.After(broadcastBlockRetryPeriod):
		}

		// check transaction
		res, err := authclient.QueryTx(cctx, hash)
		if err == nil {
			return res, nil
		}

		// if it's not a "not found" error, return
		if !strings.HasSuffix(err.Error(), notFoundErrorMessageSuffix) {
			return res, err
		}
	}

	return cres, lctx.Err()
}

func confirmTx(ctx client.Context, txb client.TxBuilder) (bool, error) {
	if ctx.SkipConfirm {
		return true, nil
	}

	out, err := ctx.TxConfig.TxJSONEncoder()(txb.GetTx())
	if err != nil {
		return false, err
	}

	_, _ = fmt.Fprintf(os.Stderr, "%s\n\n", out)

	buf := bufio.NewReader(os.Stdin)
	ok, err := input.GetConfirmation("confirm transaction before signing and broadcasting", buf, os.Stderr)

	if err != nil || !ok {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", "cancelled transaction")
		return false, err
	}

	return true, nil
}

func AdjustGas(ctx client.Context, txf tx.Factory, msgs ...sdk.Msg) (tx.Factory, error) {
	if !ctx.Simulate && !txf.SimulateAndExecute() {
		return txf, nil
	}
	_, adjusted, err := tx.CalculateGas(ctx.QueryWithData, txf, msgs...)
	if err != nil {
		return txf, err
	}

	txf = txf.WithGas(adjusted)

	return txf, nil
}

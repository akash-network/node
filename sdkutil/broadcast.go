package sdkutil

import (
	"bufio"
	"context"
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
	broadcastBlockRetryTimeout = 30 * time.Second
	broadcastBlockRetryPeriod  = time.Second

	// sadface.

	// Only way to detect the timeout error.
	// https://github.com/tendermint/tendermint/blob/46e06c97320bc61c4d98d3018f59d47ec69863c9/rpc/core/mempool.go#L124
	timeoutErrorMessage = "timed out waiting for tx to be included in a block"

	// Only way to check for tx not found error.
	// https://github.com/tendermint/tendermint/blob/46e06c97320bc61c4d98d3018f59d47ec69863c9/rpc/core/tx.go#L31-L33
	notFoundErrorMessageSuffix = ") not found"
)

func BroadcastTX(ctx client.Context, flags *pflag.FlagSet, msgs ...sdk.Msg) error {

	// rewrite of https://github.com/cosmos/cosmos-sdk/blob/ca98fda6eae597b1e7d468f96d030b6d905748d7/client/tx/tx.go#L29
	// to add continuing retries if broadcast-mode=block fails with a timeout.

	txf := tx.NewFactoryCLI(ctx, flags)

	if ctx.GenerateOnly {
		return tx.GenerateTx(ctx, txf, msgs...)
	}

	txf, err := tx.PrepareFactory(ctx, txf)
	if err != nil {
		return err
	}

	txf, err = adjustGas(ctx, txf, msgs...)
	if err != nil {
		return err
	}
	if ctx.Simulate {
		return nil
	}

	txb, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return err
	}

	ok, err := confirmTx(ctx, txb)
	if !ok || err != nil {
		return err
	}

	err = tx.Sign(txf, ctx.GetFromName(), txb, true)
	if err != nil {
		return err
	}

	txBytes, err := ctx.TxConfig.TxEncoder()(txb.GetTx())
	if err != nil {
		return err
	}

	res, err := doBroadcast(ctx, broadcastBlockRetryTimeout, txBytes)
	if err != nil {
		return err
	}

	return ctx.PrintProto(res)

}

func doBroadcast(ctx client.Context, timeout time.Duration, txb ttypes.Tx) (*sdk.TxResponse, error) {
	switch ctx.BroadcastMode {
	case flags.BroadcastSync:
		return ctx.BroadcastTxSync(txb)
	case flags.BroadcastAsync:
		return ctx.BroadcastTxAsync(txb)
	}

	// broadcast-mode=block

	// submit with mode commit/block
	cres, err := ctx.BroadcastTxCommit(txb)

	switch {
	case err == nil:
		// no error, return
		return cres, err
	case err.Error() != timeoutErrorMessage:
		// other error, return
		return cres, err
	default:
		// timeout error, continue on to retry
	}

	// loop
	lctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for lctx.Err() == nil {

		// wait up to one second
		select {
		case <-lctx.Done():
			return cres, err
		case <-time.After(broadcastBlockRetryPeriod):
		}

		// check transaction
		res, err := authclient.QueryTx(ctx, cres.TxHash)
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

func adjustGas(ctx client.Context, txf tx.Factory, msgs ...sdk.Msg) (tx.Factory, error) {
	if !ctx.Simulate && !txf.SimulateAndExecute() {
		return txf, nil
	}
	_, adjusted, err := tx.CalculateGas(ctx.QueryWithData, txf, msgs...)
	if err != nil {
		return txf, err
	}

	txf = txf.WithGas(adjusted)
	_, _ = fmt.Fprintf(os.Stderr, "%s\n", tx.GasEstimateResponse{GasEstimate: txf.Gas()})

	return txf, nil
}

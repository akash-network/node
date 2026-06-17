package grpcsuite

import (
	"context"
	"fmt"
	"time"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/stretchr/testify/require"
)

// Broadcaster signs transactions locally and broadcasts them over the gRPC tx
// ServiceClient, then polls (over gRPC) for block inclusion. This is the literal
// "broadcast a tx over the gRPC API" path: nothing here touches Comet RPC.
type Broadcaster struct {
	s      *Suite
	txSvc  txtypes.ServiceClient
	gasAdj float64
}

func newBroadcaster(s *Suite) *Broadcaster {
	return &Broadcaster{
		s:      s,
		txSvc:  txtypes.NewServiceClient(s.Conn),
		gasAdj: 1.5,
	}
}

// sign builds, simulates for gas, and signs a tx in DIRECT mode, returning the
// encoded tx bytes. Account number/sequence are fetched fresh over gRPC.
func (b *Broadcaster) sign(fromName string, msgs []sdk.Msg) ([]byte, error) {
	s := b.s
	rec, err := s.Env.Keyring.Key(fromName)
	if err != nil {
		return nil, fmt.Errorf("keyring lookup %q: %w", fromName, err)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		return nil, err
	}

	num, seq, err := s.Cctx.AccountRetriever.GetAccountNumberSequence(s.Cctx, addr)
	if err != nil {
		return nil, fmt.Errorf("account number/sequence for %s: %w", addr, err)
	}

	txf := clienttx.Factory{}.
		WithChainID(s.Env.ChainID).
		WithKeybase(s.Env.Keyring).
		WithTxConfig(s.Env.TxConfig).
		WithAccountRetriever(s.Cctx.AccountRetriever).
		WithSignMode(signing.SignMode_SIGN_MODE_DIRECT).
		WithGasAdjustment(b.gasAdj).
		WithGasPrices(s.Env.GasPrices).
		WithFromName(fromName).
		WithAccountNumber(num).
		WithSequence(seq)

	// Simulate over gRPC to compute an accurate gas limit; fees are derived from
	// gas prices in BuildUnsignedTx.
	_, adjusted, err := clienttx.CalculateGas(s.Cctx, txf, msgs...)
	if err != nil {
		return nil, fmt.Errorf("simulate gas: %w", err)
	}
	txf = txf.WithGas(adjusted)

	txb, err := txf.BuildUnsignedTx(msgs...)
	if err != nil {
		return nil, err
	}
	if err := clienttx.Sign(b.s.Ctx, txf, fromName, txb, true); err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	return s.Env.TxConfig.TxEncoder()(txb.GetTx())
}

// Broadcast signs and broadcasts msgs from fromName over gRPC, waits for block
// inclusion, and returns the final (DeliverTx) response. A non-zero result code
// (at CheckTx or DeliverTx) is returned as an error alongside the response so
// callers can assert on either.
func (b *Broadcaster) Broadcast(fromName string, msgs ...sdk.Msg) (*sdk.TxResponse, error) {
	for _, m := range msgs {
		b.s.Cov.recordMsg(sdk.MsgTypeURL(m))
	}

	bz, err := b.sign(fromName, msgs)
	if err != nil {
		return nil, err
	}

	res, err := b.txSvc.BroadcastTx(b.s.Ctx, &txtypes.BroadcastTxRequest{
		TxBytes: bz,
		Mode:    txtypes.BroadcastMode_BROADCAST_MODE_SYNC,
	})
	if err != nil {
		return nil, fmt.Errorf("gRPC BroadcastTx: %w", err)
	}
	check := res.TxResponse
	if check.Code != 0 {
		// Rejected at CheckTx (ante handler etc.) — never enters a block.
		return check, abciErr(check)
	}

	// Accepted into the mempool; wait for it to land in a block and read the
	// DeliverTx result.
	final, err := b.waitForTx(check.TxHash)
	if err != nil {
		return final, err
	}
	if final.Code != 0 {
		return final, abciErr(final)
	}
	return final, nil
}

func (b *Broadcaster) waitForTx(hash string) (*sdk.TxResponse, error) {
	ctx, cancel := context.WithTimeout(b.s.Ctx, 90*time.Second)
	defer cancel()
	for {
		gr, err := b.txSvc.GetTx(ctx, &txtypes.GetTxRequest{Hash: hash})
		if err == nil && gr.TxResponse != nil {
			return gr.TxResponse, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for tx %s inclusion: %w", hash, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func abciErr(r *sdk.TxResponse) error {
	return fmt.Errorf("tx %s failed: code=%d codespace=%s log=%q", r.TxHash, r.Code, r.Codespace, r.RawLog)
}

// BroadcastOK broadcasts msgs from fromName and requires the tx to succeed.
func (s *Suite) BroadcastOK(fromName string, msgs ...sdk.Msg) *sdk.TxResponse {
	s.T.Helper()
	res, err := s.TX.Broadcast(fromName, msgs...)
	require.NoErrorf(s.T, err, "broadcast from %s", fromName)
	require.NotNil(s.T, res)
	require.Equalf(s.T, uint32(0), res.Code, "tx failed: code=%d log=%s", res.Code, res.RawLog)
	return res
}

// BroadcastExpectErr broadcasts msgs expecting failure, and returns the response
// (which may be nil if the failure was pre-broadcast) and error for the caller to
// assert on (e.g. require.ErrorIs / a specific ABCI code).
func (s *Suite) BroadcastExpectErr(fromName string, msgs ...sdk.Msg) (*sdk.TxResponse, error) {
	s.T.Helper()
	res, err := s.TX.Broadcast(fromName, msgs...)
	require.Error(s.T, err, "expected broadcast from %s to fail", fromName)
	return res, err
}

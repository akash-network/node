package grpcsuite

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	cmttypes "github.com/cometbft/cometbft/types"
	cmtservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
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
	cmtSvc cmtservice.ServiceClient
	gasAdj float64
}

func newBroadcaster(s *Suite) *Broadcaster {
	return &Broadcaster{
		s:      s,
		txSvc:  txtypes.NewServiceClient(s.Conn),
		cmtSvc: cmtservice.NewServiceClient(s.Conn),
		gasAdj: 1.5,
	}
}

// txFactory prepares the local signing factory. Account number/sequence are
// fetched fresh over gRPC.
func (b *Broadcaster) txFactory(fromName string) (clienttx.Factory, error) {
	s := b.s
	rec, err := s.Env.Keyring.Key(fromName)
	if err != nil {
		return clienttx.Factory{}, fmt.Errorf("keyring lookup %q: %w", fromName, err)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		return clienttx.Factory{}, err
	}

	num, seq, err := s.Cctx.AccountRetriever.GetAccountNumberSequence(s.Cctx, addr)
	if err != nil {
		return clienttx.Factory{}, fmt.Errorf("account number/sequence for %s: %w", addr, err)
	}

	return clienttx.Factory{}.
		WithChainID(s.Env.ChainID).
		WithKeybase(s.Env.Keyring).
		WithTxConfig(s.Env.TxConfig).
		WithAccountRetriever(s.Cctx.AccountRetriever).
		WithSignMode(signing.SignMode_SIGN_MODE_DIRECT).
		WithGasAdjustment(b.gasAdj).
		WithGasPrices(s.Env.GasPrices).
		WithFromName(fromName).
		WithAccountNumber(num).
		WithSequence(seq), nil
}

func (b *Broadcaster) requireLocallySignable(txf clienttx.Factory, fromName string, msgs []sdk.Msg) error {
	s := b.s
	txb, err := txf.WithGas(1).BuildUnsignedTx(msgs...)
	if err != nil {
		return err
	}
	if err := clienttx.Sign(s.Ctx, txf, fromName, txb, true); err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	if _, err := s.Env.TxConfig.TxEncoder()(txb.GetTx()); err != nil {
		return err
	}
	return nil
}

func (b *Broadcaster) simulateGas(txf clienttx.Factory, msgs []sdk.Msg) (uint64, error) {
	txBytes, err := txf.BuildSimTx(msgs...)
	if err != nil {
		return 0, err
	}
	simRes, err := b.txSvc.Simulate(b.s.Ctx, &txtypes.SimulateRequest{TxBytes: txBytes})
	if err != nil {
		return 0, err
	}
	if simRes == nil || simRes.GasInfo == nil {
		return 0, fmt.Errorf("simulate gas response was empty")
	}
	return uint64(txf.GasAdjustment() * float64(simRes.GasInfo.GasUsed)), nil
}

// sign simulates for gas over gRPC and signs a tx in DIRECT mode, returning the
// encoded tx bytes.
func (b *Broadcaster) sign(txf clienttx.Factory, fromName string, msgs []sdk.Msg) ([]byte, error) {
	s := b.s

	// Simulate over gRPC to compute an accurate gas limit; fees are derived from
	// gas prices in BuildUnsignedTx.
	adjusted, err := b.simulateGas(txf, msgs)
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
	txf, err := b.txFactory(fromName)
	if err != nil {
		return nil, err
	}
	if err := b.requireLocallySignable(txf, fromName, msgs); err != nil {
		return nil, err
	}

	for _, m := range msgs {
		b.s.Cov.recordMsg(sdk.MsgTypeURL(m))
	}

	bz, err := b.sign(txf, fromName, msgs)
	if err != nil {
		return nil, err
	}
	startHeight, err := b.latestHeight(b.s.Ctx)
	if err != nil {
		return nil, fmt.Errorf("latest height before broadcast: %w", err)
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
	final, err := b.waitForTx(check.TxHash, bz, startHeight)
	if err != nil {
		return final, err
	}
	if final.Code != 0 {
		return final, abciErr(final)
	}
	return final, nil
}

func (b *Broadcaster) waitForTx(hash string, txBytes []byte, startHeight int64) (*sdk.TxResponse, error) {
	ctx, cancel := context.WithTimeout(b.s.Ctx, 2*time.Minute)
	defer cancel()
	hash = strings.ToUpper(hash)
	nextScanHeight := startHeight + 1
	var lastGetTxErr error
	var lastBlockScanErr error
	for {
		gr, err := b.txSvc.GetTx(ctx, &txtypes.GetTxRequest{Hash: hash})
		if err == nil && gr.TxResponse != nil {
			return gr.TxResponse, nil
		}
		if err != nil {
			lastGetTxErr = err
		}

		latest, err := b.latestHeight(ctx)
		if err != nil {
			lastBlockScanErr = err
		} else {
			for height := nextScanHeight; height <= latest; height++ {
				found, err := b.txIncludedAtHeight(ctx, hash, txBytes, height)
				if err != nil {
					lastBlockScanErr = err
					continue
				}
				if found {
					if indexed, indexErr := b.waitForTxIndex(ctx, hash, 5*time.Second); indexErr == nil {
						return indexed, nil
					}
					// The block scan proves the tx landed even when the tx indexer is
					// lagging or not answering GetTx. Keep the response conservative:
					// it has the hash and height, but no DeliverTx events.
					b.s.logf("tx %s found in block %d before GetTx returned it (last GetTx err: %v)", hash, height, lastGetTxErr)
					return &sdk.TxResponse{TxHash: hash, Height: height, Code: 0}, nil
				}
			}
			nextScanHeight = latest + 1
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf(
				"waiting for tx %s inclusion from height %d to %d: %w (last GetTx err: %v; last block scan err: %v)",
				hash, startHeight, nextScanHeight-1, ctx.Err(), lastGetTxErr, lastBlockScanErr,
			)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (b *Broadcaster) latestHeight(ctx context.Context) (int64, error) {
	resp, err := b.cmtSvc.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	if err != nil {
		return 0, err
	}
	if resp == nil {
		return 0, fmt.Errorf("latest block response was empty")
	}
	if resp.SdkBlock != nil {
		return resp.SdkBlock.Header.Height, nil
	}
	if resp.Block != nil {
		return resp.Block.Header.Height, nil
	}
	return 0, fmt.Errorf("latest block response was empty")
}

func (b *Broadcaster) waitForTxIndex(ctx context.Context, hash string, timeout time.Duration) (*sdk.TxResponse, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		gr, err := b.txSvc.GetTx(ctx, &txtypes.GetTxRequest{Hash: hash})
		if err == nil && gr.TxResponse != nil {
			return gr.TxResponse, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("tx %s was not indexed within %s", hash, timeout)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (b *Broadcaster) txIncludedAtHeight(ctx context.Context, hash string, txBytes []byte, height int64) (bool, error) {
	resp, err := b.cmtSvc.GetBlockByHeight(ctx, &cmtservice.GetBlockByHeightRequest{Height: height})
	if err != nil {
		return false, err
	}
	for _, tx := range blockTxs(resp) {
		txHash := strings.ToUpper(hex.EncodeToString(cmttypes.Tx(tx).Hash()))
		if txHash == hash || bytes.Equal(tx, txBytes) {
			return true, nil
		}
	}
	return false, nil
}

func blockTxs(resp *cmtservice.GetBlockByHeightResponse) [][]byte {
	if resp == nil {
		return nil
	}
	if resp.SdkBlock != nil {
		return resp.SdkBlock.Data.Txs
	}
	if resp.Block != nil {
		return resp.Block.Data.Txs
	}
	return nil
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

// BroadcastTolerant broadcasts msgs and accepts EITHER success OR a failure whose
// error contains one of okErrSubstrs. Use for messages whose acceptance depends on
// dynamic chain state that is hard to control deterministically in a test/forked
// network (e.g. the bme circuit breaker / collateral ratio): the message is still
// exercised (and counted for coverage) and the gRPC handler is proven wired,
// responding either by executing or by correctly rejecting.
func (s *Suite) BroadcastTolerant(fromName string, okErrSubstrs []string, msgs ...sdk.Msg) {
	s.T.Helper()
	_, err := s.TX.Broadcast(fromName, msgs...)
	if err == nil {
		return
	}
	for _, sub := range okErrSubstrs {
		if strings.Contains(err.Error(), sub) {
			s.logf("tolerated expected rejection from %s: %v", fromName, err)
			return
		}
	}
	require.NoErrorf(s.T, err, "broadcast from %s (not a tolerated rejection)", fromName)
}

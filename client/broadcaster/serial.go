package broadcaster

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/boz/go-lifecycle"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/tendermint/tendermint/libs/log"
	ttypes "github.com/tendermint/tendermint/types"

	"github.com/ovrclk/akash/sdkutil"
)

const (
	broadcastBlockRetryPeriod = time.Second
)

var (
	ErrNotRunning = errors.New("not running")
	// sadface.

	// Only way to detect the timeout error.
	// https://github.com/tendermint/tendermint/blob/46e06c97320bc61c4d98d3018f59d47ec69863c9/rpc/core/mempool.go#L124
	timeoutErrorMessage = "timed out waiting for tx to be included in a block"

	// Only way to check for tx not found error.
	// https://github.com/tendermint/tendermint/blob/46e06c97320bc61c4d98d3018f59d47ec69863c9/rpc/core/tx.go#L31-L33
	notFoundErrorMessageSuffix = ") not found"
)

type SerialClient interface {
	Client
	Close()
}

type seqreq struct {
	curr uint64
	ch   chan<- uint64
}

type serialBroadcaster struct {
	cctx             sdkclient.Context
	txf              tx.Factory
	info             keyring.Info
	broadcastTimeout time.Duration
	broadcastch      chan broadcastRequest
	seqreqch         chan seqreq
	lc               lifecycle.Lifecycle
	log              log.Logger
}

func NewSerialClient(log log.Logger, cctx sdkclient.Context, timeout time.Duration, txf tx.Factory, info keyring.Info) (SerialClient, error) {
	// populate account number, current sequence number
	poptxf, err := sdkutil.PrepareFactory(cctx, txf)
	if err != nil {
		return nil, err
	}

	poptxf = poptxf.WithSimulateAndExecute(true)
	client := &serialBroadcaster{
		cctx:             cctx,
		txf:              poptxf,
		info:             info,
		broadcastTimeout: timeout,
		lc:               lifecycle.New(),
		broadcastch:      make(chan broadcastRequest),
		seqreqch:         make(chan seqreq),
		log:              log.With("cmp", "client/broadcaster"),
	}

	go client.run()

	return client, nil
}

func (c *serialBroadcaster) Close() {
	c.lc.Shutdown(nil)
}

type broadcastRequest struct {
	responsech chan<- error
	msgs       []sdk.Msg
}

func (c *serialBroadcaster) Broadcast(ctx context.Context, msgs ...sdk.Msg) error {
	responsech := make(chan error, 1)
	request := broadcastRequest{
		responsech: responsech,
		msgs:       msgs,
	}

	select {
	// request received, return response
	case c.broadcastch <- request:
		return <-responsech

	// caller context cancelled, return error
	case <-ctx.Done():
		return ctx.Err()

	// loop shutting down, return error
	case <-c.lc.ShuttingDown():
		return ErrNotRunning
	}
}

func (c *serialBroadcaster) run() {
	defer c.lc.ShutdownCompleted()

	donech := make(chan struct{})

	go func() {
		defer close(donech)
		c.syncLoop()
	}()

	defer func() { <-donech }()

loop:
	for {
		select {
		case err := <-c.lc.ShutdownRequest():
			c.lc.ShutdownInitiated(err)
			break loop
		case req := <-c.broadcastch:
			// broadcast the message
			txf, err := c.broadcast(c.txf, false, req.msgs...)
			// send response
			req.responsech <- err
			c.txf = txf
		}
	}
}

func (c *serialBroadcaster) syncLoop() {
	for {
		select {
		case <-c.lc.ShuttingDown():
			return
		case req := <-c.seqreqch:
			// query sequence number
			_, seq, err := c.cctx.AccountRetriever.GetAccountNumberSequence(c.cctx, c.info.GetAddress())

			if err != nil {
				c.log.Error("error requesting account", "err", err)
				seq = req.curr
			}

			select {
			case req.ch <- seq:
			case <-c.lc.ShuttingDown():
			}
		}
	}
}

func (c *serialBroadcaster) broadcast(txf tx.Factory, retry bool, msgs ...sdk.Msg) (tx.Factory, error) {
	var err error

	if !retry {
		txf, err = sdkutil.AdjustGas(c.cctx, txf, msgs...)
		if err != nil {
			return txf, err
		}
	}

	response, err := c.doBroadcast(c.cctx, txf, c.broadcastTimeout, c.info.GetName(), msgs...)
	if err != nil {
		c.log.Error("broadcast response", "response", response, "err", err)
		return txf, err
	}

	if response.Code == 0 {
		txf = txf.WithSequence(txf.Sequence() + 1)
		return txf, nil
	}

	c.log.Error("broadcast response", "response", response)
	// transaction has failed, perform the query of account sequence to make sure correct one is used
	// for the next transaction
	ch := make(chan uint64)
	c.seqreqch <- seqreq{
		curr: txf.Sequence(),
		ch:   ch,
	}

	select {
	case curseq := <-ch:
		txf = txf.WithSequence(curseq)
	case <-c.lc.ShuttingDown():
		return txf, ErrNotRunning
	}

	if retry || (response.Code != sdkerrors.ErrWrongSequence.ABCICode()) {
		return txf, fmt.Errorf("%w: response code %d", ErrBroadcastTx, response.Code)
	}

	return c.broadcast(txf, retry, msgs...)
}

func (c *serialBroadcaster) doBroadcast(cctx sdkclient.Context, txf tx.Factory, timeout time.Duration, keyName string, msgs ...sdk.Msg) (*sdk.TxResponse, error) {
	txn, err := tx.BuildUnsignedTx(txf, msgs...)
	if err != nil {
		return nil, err
	}

	txn.SetFeeGranter(cctx.GetFeeGranterAddress())
	err = tx.Sign(txf, keyName, txn, true)
	if err != nil {
		return nil, err
	}

	bytes, err := cctx.TxConfig.TxEncoder()(txn.GetTx())
	if err != nil {
		return nil, err
	}

	txb := ttypes.Tx(bytes)
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

	// timeout error, continue on to retry

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
		// https://github.com/cosmos/cosmos-sdk/pull/8734
		res, err := authtx.QueryTx(cctx, hash)
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

package broadcaster

import (
	"context"
	"errors"
	"time"

	"github.com/boz/go-lifecycle"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	syncDuration = 10 * time.Second
)

var (
	ErrNotRunning = errors.New("not running")
)

type SerialClient interface {
	Client
	Close()
}

type serialBroadcaster struct {
	cctx        sdkclient.Context
	txf         tx.Factory
	info        keyring.Info
	broadcastch chan broadcastRequest
	lc          lifecycle.Lifecycle
	log         log.Logger
}

func NewSerialClient(log log.Logger, cctx sdkclient.Context, txf tx.Factory, info keyring.Info) (SerialClient, error) {

	// populate account number, current sequence number
	poptxf, err := tx.PrepareFactory(cctx, txf)
	if err != nil {
		return nil, err
	}

	client := &serialBroadcaster{
		cctx:        cctx,
		txf:         poptxf,
		info:        info,
		lc:          lifecycle.New(),
		broadcastch: make(chan broadcastRequest),
		log:         log.With("cmp", "client/broadcaster"),
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

	var (
		txf    = c.txf
		synch  = make(chan uint64)
		donech = make(chan struct{})
	)

	go func() {
		defer close(donech)
		c.syncLoop(synch)
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
			err := doBroadcast(c.cctx, txf, c.info.GetName(), req.msgs...)

			// send response
			// TODO: respond to "sequence mismatch" errors here.  not sure how to detect them.
			req.responsech <- err

			if err != nil {
				c.log.Error("request error", "sequence", txf.Sequence(), "err", err)
			}

			// update our sequence number
			txf = txf.WithSequence(txf.Sequence() + 1)

		case seqno := <-synch:

			c.log.Info("syncing sequence", "local", txf.Sequence(), "remote", seqno)

			// fast-forward current sequence if necessary
			if seqno > txf.Sequence() {
				txf = txf.WithSequence(seqno)
			}
		}
	}
}

func (c *serialBroadcaster) syncLoop(ch chan<- uint64) {
	// TODO: add jitter, force update on "sequence mismatch"-type errors.
	ticker := time.NewTicker(syncDuration)
	defer ticker.Stop()

	for {
		select {
		case <-c.lc.ShuttingDown():
			return
		case <-ticker.C:

			// query sequence number
			_, seq, err := c.cctx.AccountRetriever.
				GetAccountNumberSequence(c.cctx, c.info.GetAddress())

			// send to main loop if no error
			if err != nil {
				c.log.Error("error requesting account", "err", err)
				break
			}

			select {
			case ch <- seq:
			case <-c.lc.ShuttingDown():
			}

		}
	}
}

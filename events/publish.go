package events

import (
	"context"

	"github.com/boz/go-lifecycle"
	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"

	atypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	mtypes "github.com/akash-network/akash-api/go/node/market/v1beta4"
	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"
	"github.com/akash-network/akash-api/go/sdkutil"

	"github.com/akash-network/node/pubsub"
)

type events struct {
	ctx    context.Context
	group  *errgroup.Group
	client tmclient.Client
	bus    pubsub.Bus
	lc     lifecycle.Lifecycle
}

// Service represents an event monitoring service that subscribes to and processes blockchain events.
// It monitors block headers and various transaction events, publishing them to a message bus.
type Service interface {
	// Shutdown gracefully stops the event monitoring service and cleans up resources.
	// Once called, the service will unsubscribe from events and complete any pending operations.
	Shutdown()
}

// NewEvents creates and initializes a new blockchain event monitoring service.
//
// Parameters:
//   - pctx: Parent context for controlling the service lifecycle
//   - client: Tendermint RPC client for interacting with the blockchain
//   - name: Service name used as a prefix for subscription identifiers
//   - bus: Message bus for publishing processed events
//
// Returns:
//   - Service: A running event monitoring service interface
//   - error: Any error encountered during service initialization
//
// The service subscribes to block header events and processes them to extract and publish
// various transaction events (deployment, market, provider, audit) to the provided message bus.
// The service starts monitoring events immediately and will continue until either the context
// is canceled or Shutdown() is called.
func NewEvents(pctx context.Context, client tmclient.Client, name string, bus pubsub.Bus) (Service, error) {
	group, ctx := errgroup.WithContext(pctx)

	ev := &events{
		ctx:    ctx,
		group:  group,
		client: client,
		lc:     lifecycle.New(),
		bus:    bus,
	}

	const (
		queuesz = 1000
	)

	var blkHeaderName = name + "-blk-hdr"

	tmbus := client.(tmclient.EventsClient)

	blkch, err := tmbus.Subscribe(ctx, blkHeaderName, blkHeaderQuery().String(), queuesz)
	if err != nil {
		return nil, err
	}

	startch := make(chan struct{}, 1)

	group.Go(func() error {
		ev.lc.WatchContext(ctx)

		return ev.lc.Error()
	})

	group.Go(func() error {
		return ev.run(blkHeaderName, blkch, startch)
	})

	select {
	case <-pctx.Done():
		return nil, pctx.Err()
	case <-startch:
		return ev, nil
	}
}

func (e *events) Shutdown() {
	_, stopped := <-e.lc.Done()
	if stopped {
		return
	}

	e.lc.Shutdown(nil)

	_ = e.group.Wait()
}

func (e *events) run(subs string, ch <-chan ctypes.ResultEvent, startch chan<- struct{}) error {
	tmbus := e.client.(tmclient.EventsClient)

	defer func() {
		_ = tmbus.UnsubscribeAll(e.ctx, subs)

		e.lc.ShutdownCompleted()
	}()

	startch <- struct{}{}

loop:
	for {
		select {
		case err := <-e.lc.ShutdownRequest():
			e.lc.ShutdownInitiated(err)
			break loop
		case ev := <-ch:
			// nolint: gocritic
			switch evt := ev.Data.(type) {
			case tmtmtypes.EventDataNewBlockHeader:
				e.processBlock(evt.Header.Height)
			}
		}
	}

	return e.ctx.Err()
}

func (e *events) processBlock(height int64) {
	blkResults, err := e.client.BlockResults(e.ctx, &height)
	if err != nil {
		return
	}

	for _, tx := range blkResults.TxsResults {
		if tx == nil {
			continue
		}

		for _, ev := range tx.Events {
			if mev, ok := processEvent(ev); ok {
				if err := e.bus.Publish(mev); err != nil {
					return
				}
			}
		}
	}
}

func processEvent(bev abci.Event) (interface{}, bool) {
	ev, err := sdkutil.ParseEvent(sdk.StringifyEvent(bev))
	if err != nil {
		return nil, false
	}

	if mev, err := dtypes.ParseEvent(ev); err == nil {
		return mev, true
	}

	if mev, err := mtypes.ParseEvent(ev); err == nil {
		return mev, true
	}

	if mev, err := ptypes.ParseEvent(ev); err == nil {
		return mev, true
	}

	if mev, err := atypes.ParseEvent(ev); err == nil {
		return mev, true
	}

	return nil, false
}

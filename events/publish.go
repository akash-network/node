package events

import (
	"context"

	"github.com/boz/go-lifecycle"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"golang.org/x/sync/errgroup"
	atypes "pkg.akt.dev/go/node/audit/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"

	abci "github.com/cometbft/cometbft/abci/types"
	cmclient "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"

	"pkg.akt.dev/node/pubsub"
)

type events struct {
	ctx    context.Context
	group  *errgroup.Group
	ebus   cmclient.EventsClient
	client sdkclient.CometRPC
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
func NewEvents(pctx context.Context, node sdkclient.CometRPC, name string, bus pubsub.Bus) (Service, error) {
	group, ctx := errgroup.WithContext(pctx)

	ev := &events{
		ctx:    ctx,
		group:  group,
		ebus:   node.(cmclient.EventsClient),
		client: node,
		lc:     lifecycle.New(),
		bus:    bus,
	}

	const (
		queuesz = 1000
	)

	var blkHeaderName = name + "-blk-hdr"

	blkch, err := ev.ebus.Subscribe(ctx, blkHeaderName, blkHeaderQuery().String(), queuesz)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = ev.ebus.UnsubscribeAll(ctx, blkHeaderName)
	}()

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
	defer func() {
		_ = e.ebus.UnsubscribeAll(e.ctx, subs)

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
			case cmtypes.EventDataNewBlockHeader:
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
	pev, err := sdk.ParseTypedEvent(bev)
	if err != nil {
		return nil, false
	}

	switch pev.(type) {
	case *atypes.EventTrustedAuditorCreated:
	case *atypes.EventTrustedAuditorDeleted:
	case *dtypes.EventDeploymentCreated:
	case *dtypes.EventDeploymentUpdated:
	case *dtypes.EventDeploymentClosed:
	case *dtypes.EventGroupStarted:
	case *dtypes.EventGroupPaused:
	case *dtypes.EventGroupClosed:
	case *mtypes.EventOrderCreated:
	case *mtypes.EventOrderClosed:
	case *mtypes.EventBidCreated:
	case *mtypes.EventBidClosed:
	case *mtypes.EventLeaseCreated:
	case *mtypes.EventLeaseClosed:
	default:
		return nil, false
	}

	return bev, true
}

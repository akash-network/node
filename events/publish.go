package events

import (
	"context"

	"github.com/cosmos/gogoproto/proto"
	"golang.org/x/sync/errgroup"
	atypes "pkg.akt.dev/go/node/audit/v1"
	dtypes "pkg.akt.dev/go/node/deployment/v1"
	mtypes "pkg.akt.dev/go/node/market/v1"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmclient "github.com/cometbft/cometbft/rpc/client"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmtypes "github.com/cometbft/cometbft/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/akashd/pubsub"
)

// Publish events using cometbft buses to clients. Waits on context
func Publish(ctx context.Context, evbus cmclient.EventsClient, name string, bus pubsub.Bus) (err error) {
	const queuesz = 100
	var txname = name + "-tx"
	var blkname = name + "-blk"

	txch, err := evbus.Subscribe(ctx, txname, txQuery().String(), queuesz)
	if err != nil {
		return err
	}
	defer func() {
		err = evbus.UnsubscribeAll(ctx, txname)
	}()

	blkch, err := evbus.Subscribe(ctx, blkname, blkQuery().String(), queuesz)
	if err != nil {
		return err
	}
	defer func() {
		err = evbus.UnsubscribeAll(ctx, blkname)
	}()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return publishEvents(ctx, txch, bus)
	})

	g.Go(func() error {
		return publishEvents(ctx, blkch, bus)
	})

	return g.Wait()
}

func publishEvents(ctx context.Context, ch <-chan ctypes.ResultEvent, bus pubsub.Bus) error {
	defer bus.Close()

	var err error

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case ed := <-ch:
			switch evt := ed.Data.(type) {
			case cmtypes.EventDataTx:
				if !evt.Result.IsOK() {
					continue
				}

				if err = processEvents(bus, evt.Result.GetEvents()); err != nil {
					return err
				}
			case cmtypes.EventDataNewBlockHeader:
				if err = processEvents(bus, evt.ResultEndBlock.GetEvents()); err != nil {
					return err
				}
			}
		}
	}

	return err
}

func processEvents(bus pubsub.Bus, events []abcitypes.Event) error {
	for _, ev := range events {
		evt, err := sdktypes.ParseTypedEvent(ev)
		if err != nil {
			continue
		}

		if evt = filterEvent(evt); evt == nil {
			continue
		}

		if err := bus.Publish(evt); err != nil {
			return err
		}
	}

	return nil
}

func filterEvent(bev proto.Message) proto.Message {
	switch bev.(type) {
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
		return nil
	}

	return bev
}

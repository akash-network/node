package events

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/sdkutil"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
	abci "github.com/tendermint/tendermint/abci/types"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"
)

// Publish events using tm buses to clients. Waits on context
// shutdown signals to exit.
func Publish(ctx context.Context, tmbus tmclient.EventsClient, name string, bus pubsub.Bus) error {

	const (
		queuesz = 100
	)
	var (
		txname  = name + "-tx"
		blkname = name + "-blk"
	)

	txch, err := tmbus.Subscribe(ctx, txname, txQuery().String(), queuesz)
	if err != nil {
		return err
	}
	defer tmbus.UnsubscribeAll(ctx, txname)

	blkch, err := tmbus.Subscribe(ctx, blkname, blkQuery().String(), queuesz)
	if err != nil {
		return err
	}
	defer tmbus.UnsubscribeAll(ctx, txname)

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
	var err error

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case ed := <-ch:
			switch evt := ed.Data.(type) {
			case tmtmtypes.EventDataTx:
				if !evt.Result.IsOK() {
					continue
				}
				processEvents(bus, evt.Result.GetEvents())
			case tmtmtypes.EventDataNewBlockHeader:
				processEvents(bus, evt.ResultEndBlock.GetEvents())
			}
		}
	}

	return err
}

func processEvents(bus pubsub.Bus, events []abci.Event) {
	for _, ev := range events {
		if mev, ok := processEvent(ev); ok {
			if err := bus.Publish(mev); err != nil {
				return
			}
			continue
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

	return nil, false
}

package events

import (
	"context"

	"golang.org/x/sync/errgroup"

	sdk "github.com/cosmos/cosmos-sdk/types"
	abci "github.com/tendermint/tendermint/abci/types"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtmtypes "github.com/tendermint/tendermint/types"

	atypes "github.com/akash-network/akash-api/go/node/audit/v1beta3"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
	mtypes "github.com/akash-network/akash-api/go/node/market/v1beta4"
	ptypes "github.com/akash-network/akash-api/go/node/provider/v1beta3"
	"github.com/akash-network/akash-api/go/sdkutil"

	"github.com/akash-network/node/pubsub"
)

// Publish events using tm buses to clients. Waits on context
// shutdown signals to exit.
func Publish(ctx context.Context, tmbus tmclient.EventsClient, name string, bus pubsub.Bus) (err error) {

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
	defer func() {
		err = tmbus.UnsubscribeAll(ctx, txname)
	}()

	blkch, err := tmbus.Subscribe(ctx, blkname, blkQuery().String(), queuesz)
	if err != nil {
		return err
	}
	defer func() {
		err = tmbus.UnsubscribeAll(ctx, blkname)
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
				bus.Close()
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

	if mev, err := atypes.ParseEvent(ev); err == nil {
		return mev, true
	}

	return nil, false
}

package events

import (
	"context"
	"github.com/cosmos/cosmos-sdk/client/polling"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/sdkutil"
	atypes "github.com/ovrclk/akash/x/audit/types/v1beta2"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"golang.org/x/sync/errgroup"
)

const queueSize = 100

// Publish events using tm buses to clients. Waits on context
// shutdown signals to exit.
func Publish(ctx context.Context, tmbus tmclient.EventsClient, name string, bus pubsub.Bus) (err error) {
	var (
		txname  = name + "-tx"
		blkname = name + "-blk"
	)

	txch, err := tmbus.Subscribe(ctx, txname, txQuery().String(), queueSize)
	if err != nil {
		return err
	}
	defer func() {
		err = tmbus.UnsubscribeAll(ctx, txname)
	}()

	blkch, err := tmbus.Subscribe(ctx, blkname, blkQuery().String(), queueSize)
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

func PublishFromPolling(ctx context.Context, logger log.Logger, c tmclient.Client, bus pubsub.Bus) error {
	transactionsChannel, err := polling.PollForBlocks(ctx, logger, c, queueSize)
	if err != nil {
		return err
	}

	return publishEventsFrom(ctx, transactionsChannel, bus)
}

func publishEventsFrom(ctx context.Context, ch <-chan abci.ResponseDeliverTx, bus pubsub.Bus) error {
	for {
		var txn abci.ResponseDeliverTx
		select {
		case txn = <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}

		if txn.Code != 0 {
			continue
		}
		for _, ev := range txn.Events {
			if mev, ok := processEvent(ev); ok {
				if err := bus.Publish(mev); err != nil {
					bus.Close()
					return err
				}
			}
		}
	}
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

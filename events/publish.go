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
func Publish(ctx context.Context, client tmclient.Client, name string, bus pubsub.Bus) (err error) {
	const (
		queuesz = 1000
	)
	var (
		blkHeaderName = name + "-blk-hdr"
	)

	tmbus := client.(tmclient.EventsClient)

	blkch, err := tmbus.Subscribe(ctx, blkHeaderName, blkHeaderQuery().String(), queuesz)
	if err != nil {
		return err
	}
	defer func() {
		err = tmbus.UnsubscribeAll(ctx, blkHeaderName)
	}()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return publishEvents(ctx, client, blkch, bus)
	})

	return g.Wait()
}

func publishEvents(ctx context.Context, client tmclient.Client, ch <-chan ctypes.ResultEvent, bus pubsub.Bus) error {
	var err error

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case ed := <-ch:
			// nolint: gocritic
			switch evt := ed.Data.(type) {
			case tmtmtypes.EventDataNewBlockHeader:
				processBlock(ctx, bus, client, evt.Header.Height)
			}
		}
	}

	return err
}

func processBlock(ctx context.Context, bus pubsub.Bus, client tmclient.Client, height int64) {
	blkResults, err := client.BlockResults(ctx, &height)
	if err != nil {
		return
	}

	for _, tx := range blkResults.TxsResults {
		if tx == nil {
			continue
		}

		for _, ev := range tx.Events {
			if mev, ok := processEvent(ev); ok {
				if err := bus.Publish(mev); err != nil {
					bus.Close()
					return
				}
				continue
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

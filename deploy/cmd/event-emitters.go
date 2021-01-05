package cmd

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ovrclk/akash/events"
	"github.com/ovrclk/akash/pubsub"
	"golang.org/x/sync/errgroup"
)

// EventEmitter is a type that describes event emitter functions
type EventEmitter func(context.Context, ...EventHandler) error

// ChainEmitter runs the passed EventHandlers just on the on chain event stream
func ChainEmitter(ctx context.Context, clientCtx client.Context, ehs ...EventHandler) (err error) {
	// Instantiate and start tendermint RPC client
	if err = clientCtx.Client.Start(); err != nil {
		return err
	}

	// Start the pubsub bus
	bus := pubsub.NewBus()
	defer bus.Close()

	// Initialize a new error group
	group, ctx := errgroup.WithContext(ctx)

	// Publish chain events to the pubsub bus
	group.Go(func() error {
		return events.Publish(ctx, clientCtx.Client, "akash-deploy", bus)
	})

	// Subscribe to the bus events
	subscriber, err := bus.Subscribe()
	if err != nil {
		return err
	}

	// Handle all the events coming out of the bus
	group.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-subscriber.Done():
				return nil
			case ev := <-subscriber.Events():
				for _, eh := range ehs {
					if err = eh(ev); err != nil {
						return err
					}
				}
			}
		}
	})

	return group.Wait()
}

package common

import (
	"context"

	"github.com/ovrclk/akash/marketplace"
	"github.com/tendermint/tendermint/libs/log"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func MonitorMarketplace(ctx context.Context, log log.Logger, client *tmclient.HTTP, handler marketplace.Handler) error {
	ctx, cancel := context.WithCancel(ctx)

	if err := client.Start(); err != nil {
		log.Error("error starting ws client", "error", err)
		cancel()
		return err
	}

	cdonech := make(chan interface{})
	defer func() { <-cdonech }()
	go func() {
		defer close(cdonech)
		client.Wait()
	}()

	monitor, err := marketplace.NewMonitor(ctx, log, client, "akash-cli", handler, marketplace.TxQuery())
	if err != nil {
		cancel()
		return err
	}
	defer func() { <-monitor.Wait() }()

	select {
	case <-ctx.Done():
		client.UnsubscribeAll(context.Background(), "akash-cli")
		client.Stop()
	case <-cdonech:
	}

	cancel()
	return nil
}

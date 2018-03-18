package common

import (
	"context"

	"github.com/ovrclk/akash/marketplace"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tmlibs/log"
)

func MonitorMarketplace(log log.Logger, client *tmclient.HTTP, handler marketplace.Handler) error {
	return doMonitorMarketplace(context.Background(), log, client, handler)
}

func doMonitorMarketplace(ctx context.Context, log log.Logger, client *tmclient.HTTP, handler marketplace.Handler) error {

	ctx, cancel := context.WithCancel(ctx)
	donech := WatchSignals(ctx, cancel)
	defer func() {
		cancel()
		<-donech
	}()

	if err := client.Start(); err != nil {
		log.Error("error starting ws client", err)
		return err
	}

	cdonech := make(chan interface{})

	go func() {
		defer close(cdonech)
		client.Wait()
	}()

	monitor, err := marketplace.NewMonitor(ctx, log, client, "akash-cli", handler, marketplace.TxQuery())
	if err != nil {
		return err
	}
	defer func() {
		<-monitor.Wait()
	}()

	select {
	case <-ctx.Done():
		client.UnsubscribeAll(context.Background(), "akash-cli")
		client.Stop()
	case <-cdonech:
	}

	return nil
}

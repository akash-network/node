package common

import (
	"context"

	"github.com/ovrclk/photon/marketplace"
	tmclient "github.com/tendermint/tendermint/rpc/client"
	"github.com/tendermint/tmlibs/log"
)

func MonitorMarketplace(log log.Logger, client *tmclient.HTTP, handler marketplace.Handler) error {
	ctx, cancel := context.WithCancel(context.Background())
	donech := WatchSignals(ctx, cancel)

	monitor := marketplace.NewMonitor(ctx, log, client)

	if err := monitor.Start(); err != nil {
		log.Error("error starting monitor", err)
		return err
	}

	monitor.AddHandler("photon-cli", handler, marketplace.TxQuery())

	<-monitor.Wait()
	cancel()
	<-donech
	return nil
}

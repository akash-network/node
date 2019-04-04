package common

import (
	"context"
	"fmt"

	"github.com/ovrclk/akash/marketplace"
	tmlog "github.com/tendermint/tendermint/libs/log"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func MonitorMarketplace(ctx context.Context, log tmlog.Logger, client *tmclient.HTTP, handler marketplace.Handler) error {
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
		fmt.Println("cmn.Monitor: waiting on client to close")
		client.Wait()
	}()

	monitor, err := marketplace.NewMonitor(ctx, log, client, "akash-cli", handler, marketplace.TxQuery())
	if err != nil {
		cancel()
		return err
	}
	defer func() {
		fmt.Println("cmn.Monitor: waiting on monitor to close")
		<-monitor.Wait()
		fmt.Println("cmn.Monitor: no wait monitor stopped")
	}()

	select {
	case <-ctx.Done():
		monitor.Stop()
		client.UnsubscribeAll(context.Background(), "akash-cli")
		client.Stop()
	case <-cdonech:
		fmt.Println("cmn.Monitor: cdonech closed")
	}

	fmt.Println("cmn.Monitor: the end")
	cancel()
	return nil
}

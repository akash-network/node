package main

import (
	"fmt"

	goctx "context"

	"github.com/ovrclk/photon/cmd/common"
	"github.com/ovrclk/photon/cmd/photon/context"
	"github.com/ovrclk/photon/marketplace"
	"github.com/ovrclk/photon/types"
	"github.com/spf13/cobra"
	tmclient "github.com/tendermint/tendermint/rpc/client"
)

func marketplaceCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "monitor marketplace",
		Args:  cobra.NoArgs,
		RunE:  context.WithContext(context.RequireNode(doMarketplaceMonitorCommand)),
	}

	context.AddFlagNode(cmd, cmd.PersistentFlags())

	return cmd
}

func doMarketplaceMonitorCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	client := tmclient.NewHTTP(ctx.Node(), "/websocket")

	gctx, cancel := goctx.WithCancel(goctx.Background())
	donech := common.WatchSignals(gctx, cancel)

	m := marketplace.NewMonitor(gctx, ctx.Log(), client)
	h := marketplaceMonitorHandler()

	if err := m.Start(); err != nil {
		return err
	}

	m.AddHandler("photon-cli", h, marketplace.TxQuery())

	<-m.Wait()
	cancel()
	<-donech

	return nil
}

func marketplaceMonitorHandler() marketplace.Handler {
	return marketplace.NewBuilder().
		OnTxSend(func(tx *types.TxSend) {
			fmt.Printf("TRANSFER %v tokens from %X to %X\n", tx.GetAmount(), tx.From, tx.To)
		}).Create()
}

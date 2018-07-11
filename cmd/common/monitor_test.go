package common

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/testutil"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/abci/example/counter"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/rpc/client"
	rpctest "github.com/tendermint/tendermint/rpc/test"
)

func TestMonitorMarketplace(t *testing.T) {
	node := startTestServer(t)
	defer stopTestServer(t, node)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := getTestHTTPClient()

	handler := marketplace.NewBuilder().Create()

	go func() {
		defer cancel()
		testutil.SleepForThreadStart(t)
	}()

	require.NoError(t, MonitorMarketplace(ctx, testutil.Logger(), client, handler))

	<-ctx.Done()
}

func getTestHTTPClient() *client.HTTP {
	rpcAddr := rpctest.GetConfig().RPC.ListenAddress
	return client.NewHTTP(rpcAddr, "/websocket")
}

func startTestServer(t *testing.T) *node.Node {
	app := counter.NewCounterApplication(true)
	node := rpctest.StartTendermint(app)
	return node
}

func stopTestServer(t *testing.T, node *node.Node) {
	require.NoError(t, node.Stop())
	node.Wait()
}

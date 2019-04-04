package marketplace_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/marketplace/mocks"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/abci/example/counter"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/rpc/client"
	rpctest "github.com/tendermint/tendermint/rpc/test"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

func getLocalClient(node *node.Node) *client.Local {
	return client.NewLocal(node)
}

func getTestHTTPClient() *client.HTTP {
	rpcAddr := rpctest.GetConfig().RPC.ListenAddress
	return client.NewHTTP(rpcAddr, "/websocket")
}

func startTestServer(t *testing.T) *node.Node {
	app := counter.NewCounterApplication(true)
	node := rpctest.StartTendermint(app)
	fmt.Println("start test server")
	return node
}

func stopTestServer(t *testing.T, node *node.Node) {
	fmt.Println("stop test server")
	require.NoError(t, node.Stop())
	fmt.Println("stopped test server, waiting on node stop")
	node.Wait()
	fmt.Println("stopped test server, waiting on node stopped")
}

func TestMonitorMarketplace(t *testing.T) {
	node := startTestServer(t)
	defer stopTestServer(t, node)
	lc := getLocalClient(node)
	bus := lc.EventBus

	signer, _ := testutil.PrivateKeySigner(t)

	tests := []struct {
		name    string
		payload interface{}
	}{
		{"OnTxSend", &types.TxSend{}},
		{"OnTxCreateProvider", &types.TxCreateProvider{}},
		{"OnTxCreateDeployment", &types.TxCreateDeployment{}},
		{"OnTxCreateOrder", &types.TxCreateOrder{}},
		{"OnTxCreateFulfillment", &types.TxCreateFulfillment{}},
		{"OnTxCreateLease", &types.TxCreateLease{}},
		{"OnTxCloseDeployment", &types.TxCloseDeployment{}},
		{"OnTxCloseFulfillment", &types.TxCloseFulfillment{}},
		{"OnTxCloseLease", &types.TxCloseLease{}},
	}

	ctx := context.Background()

	for _, test := range tests {
		fmt.Println("running test", test)

		h := new(mocks.Handler)
		h.On(test.name, test.payload).Return(nil).Once()

		m, err := marketplace.NewMonitor(ctx, testutil.Logger(), lc, t.Name(), h, marketplace.TxQuery())
		if !assert.NoError(t, err, test.name) {
			continue
		}

		tx, err := txutil.BuildTx(signer, 1, test.payload)
		if !assert.NoError(t, err, test.name) {
			continue
		}

		bus.PublishEventTx(tmtmtypes.EventDataTx{
			TxResult: tmtmtypes.TxResult{
				Tx: tx,
			},
		})

		testutil.SleepForThreadStart(t)

		if !assert.NoError(t, m.Stop()) {
			continue
		}

		h.AssertExpectations(t)
	}

}

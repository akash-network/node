package market

import (
	"github.com/tendermint/tendermint/rpc/core"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

type Client interface {
	BroadcastTxAsync(tx tmtmtypes.Tx) (*ctypes.ResultBroadcastTx, error)
}

// local mempool client
func newLocalClient() Client {
	return localClient{}
}

type localClient struct{}

func (localClient) BroadcastTxAsync(tx tmtmtypes.Tx) (*ctypes.ResultBroadcastTx, error) {
	return core.BroadcastTxAsync(tx)
}

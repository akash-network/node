package types

import (
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	tmtypes "github.com/tendermint/abci/types"
)

type Application interface {
	AcceptQuery(req tmtypes.RequestQuery) bool
	Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery

	AcceptTx(ctx Context, tx interface{}) bool
	CheckTx(ctx Context, tx interface{}) tmtypes.ResponseCheckTx
	DeliverTx(ctx Context, tx interface{}) tmtypes.ResponseDeliverTx
}

type Context interface {
	Signer() base.PubKey
}

func NewContext(tx *types.Tx) Context {
	return &context{tx}
}

type context struct {
	tx *types.Tx
}

func (ctx context) Signer() base.PubKey {
	return *ctx.tx.Key
}

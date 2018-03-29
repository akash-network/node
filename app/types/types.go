package types

import (
	"github.com/ovrclk/akash/types"
	tmtypes "github.com/tendermint/abci/types"
	crypto "github.com/tendermint/go-crypto"
)

type Application interface {
	Name() string
	AcceptQuery(req tmtypes.RequestQuery) bool
	Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery

	AcceptTx(ctx Context, tx interface{}) bool
	CheckTx(ctx Context, tx interface{}) tmtypes.ResponseCheckTx
	DeliverTx(ctx Context, tx interface{}) tmtypes.ResponseDeliverTx
}

type Context interface {
	Signer() crypto.PubKey
}

func NewContext(tx *types.Tx) Context {
	return &context{tx}
}

type context struct {
	tx *types.Tx
}

func (ctx context) Signer() crypto.PubKey {
	key, err := crypto.PubKeyFromBytes(ctx.tx.Key)
	// XXX handle errors
	if err != nil {
		panic(err)
	}
	return key
}

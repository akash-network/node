package types

import (
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	abci_types "github.com/tendermint/tendermint/abci/types"
	crypto "github.com/tendermint/tendermint/crypto"
	tmcamino "github.com/tendermint/tendermint/crypto/encoding/amino"
)

type Application interface {
	Name() string
	AcceptQuery(req abci_types.RequestQuery) bool
	Query(state state.State, req abci_types.RequestQuery) abci_types.ResponseQuery

	AcceptTx(ctx Context, tx interface{}) bool
	CheckTx(state state.State, ctx Context, tx interface{}) abci_types.ResponseCheckTx
	DeliverTx(state state.State, ctx Context, tx interface{}) abci_types.ResponseDeliverTx
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
	key, err := tmcamino.PubKeyFromBytes(ctx.tx.Key)

	// // XXX handle errors
	if err != nil {
		panic(err)
	}
	return key
}

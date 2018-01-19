package accounts

import (
	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/errors"
	"github.com/cosmos/cosmos-sdk/state"
	wire "github.com/tendermint/go-wire"
)

const (
	// Name of the module for registering it
	Name = "accounts"

	// CostSet is the gas needed for the set operation
	CostSet int64 = 0
	// CostRemove is the gas needed for the remove operation
	CostRemove = 0
)

// Handler allows us to set and remove data
type Handler struct {
	sdk.NopInitState
	sdk.NopInitValidate
}

var _ sdk.Handler = Handler{}

// NewHandler makes a role handler to modify data
func NewHandler() Handler {
	return Handler{}
}

// Name - return name space
func (Handler) Name() string {
	return Name
}

// CheckTx verifies if the transaction is properly formated
func (h Handler) CheckTx(ctx sdk.Context, store state.SimpleDB, tx sdk.Tx) (res sdk.CheckResult, err error) {
	err = tx.ValidateBasic()
	return
}

// DeliverTx tries to create a new role.
func (h Handler) DeliverTx(ctx sdk.Context, store state.SimpleDB, tx sdk.Tx) (res sdk.DeliverResult, err error) {
	err = tx.ValidateBasic()
	if err != nil {
		return
	}

	switch t := tx.Unwrap().(type) {
	case SetTx:
		res, err = h.doSetTx(ctx, store, t)
	case RemoveTx:
		res, err = h.doRemoveTx(ctx, store, t)
	// case CreateTx:
	// 	res, err = h.doCreateTx(ctx, store, t)
	default:
		err = errors.ErrUnknownTxType(tx)
	}
	return
}

// doSetTx writes to the store, overwriting any previous value
// note that an empty response in DeliverTx is OK with no log or data returned
func (h Handler) doSetTx(ctx sdk.Context, store state.SimpleDB, tx SetTx) (res sdk.DeliverResult, err error) {
	data := NewData(tx.Value, ctx.BlockHeight())
	store.Set(tx.Key, wire.BinaryBytes(data))
	return
}

// doRemoveTx deletes the value from the store and returns the last value
// here we let res.Data to return the value over abci
func (h Handler) doRemoveTx(ctx sdk.Context, store state.SimpleDB, tx RemoveTx) (res sdk.DeliverResult, err error) {
	// we set res.Data so it gets returned to the client over the abci interface
	res.Data = store.Get(tx.Key)
	if len(res.Data) != 0 {
		store.Remove(tx.Key)
	}
	return
}

// func (h Handler) doCreateTx(ctx sdk.Context, store state.SimpleDB, tx CreateTx) (res sdk.DeliverResult, err error) {
// 	data := NewData(tx.Type, ctx.BlockHeight())

// 	// todo: get tx signer address
// 	// print("context")
// 	// print(ctx)

// 	address := []byte("0x01")

// 	store.Set(address, wire.BinaryBytes(data))
// 	return
// }

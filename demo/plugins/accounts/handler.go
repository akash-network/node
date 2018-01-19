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
	case CreateTx:
		res, err = h.doCreateTx(ctx, store, t)
	default:
		err = errors.ErrUnknownTxType(tx)
	}
	return
}

func (h Handler) doCreateTx(ctx sdk.Context, store state.SimpleDB, tx CreateTx) (res sdk.DeliverResult, err error) {
	// check if tx has permission
	if !ctx.HasPermission(tx.Actor) {
		err = errors.ErrUnauthorized()
		return
	}
	data := NewData(tx.Type, ctx.BlockHeight())
	store.Set(tx.Actor.Address, wire.BinaryBytes(data))
	return
}

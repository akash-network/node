package account

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	Name = apptypes.TagAppAccount
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, logger)}, nil
}

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.AccountPath)
}

func (a *app) Query(state appstate.State, req abci_types.RequestQuery) abci_types.ResponseQuery {

	if !a.AcceptQuery(req) {
		return abci_types.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
	}
	id := strings.TrimPrefix(req.Path, appstate.AccountPath)
	key, err := keys.ParseAccountPath(id)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	acct, err := state.Account().Get(key.ID())
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if acct == nil {
		return abci_types.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("account %x not found", key),
		}
	}

	bytes, err := proto.Marshal(acct)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxSend:
		return true
	}
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxSend:
		return a.doCheckTx(state, ctx, tx.TxSend)
	}
	return abci_types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxSend:
		return a.doDeliverTx(state, ctx, tx.TxSend)
	}
	return abci_types.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) doCheckTx(state appstate.State, ctx apptypes.Context, tx *types.TxSend) abci_types.ResponseCheckTx {

	if !bytes.Equal(ctx.Signer().Address(), tx.From) {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by sending address",
		}
	}

	if bytes.Equal(tx.From, tx.To) {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "source and destination can't be the same address",
		}
	}

	acct, err := state.Account().Get(tx.From)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if acct == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "unknown source account",
		}
	}

	if acct.Balance < tx.Amount {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "insufficient funds",
		}
	}

	return abci_types.ResponseCheckTx{}
}

func (a *app) doDeliverTx(state appstate.State, ctx apptypes.Context, tx *types.TxSend) abci_types.ResponseDeliverTx {

	cresp := a.doCheckTx(state, ctx, tx)
	if !cresp.IsOK() {
		return abci_types.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	acct, err := state.Account().Get(tx.From)
	if err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if acct == nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "unknown source account",
		}
	}

	toacct, err := state.Account().Get(tx.To)
	if err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	if toacct == nil {
		toacct = &types.Account{
			Address: tx.To,
		}
	}

	acct.Balance -= tx.Amount
	toacct.Balance += tx.Amount

	if err := state.Account().Save(acct); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	if err := state.Account().Save(toacct); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeSend),
	}
}

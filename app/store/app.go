package store

import (
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"

	apptypes "github.com/ovrclk/akash/app/types"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types/code"
)

const (
	QueryPath = "/store"
	Name      = "store"
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, logger)}, nil
}

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return req.Path == QueryPath
}

func (a *app) Query(state appstate.State, req abci_types.RequestQuery) abci_types.ResponseQuery {
	if !a.AcceptQuery(req) {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  "invalid query",
		}
	}

	val := state.Get(req.Data)
	return abci_types.ResponseQuery{
		Value:  val,
		Height: state.Version(),
	}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	return abci_types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "store app: unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	return abci_types.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "store app: unknown transaction",
	}
}

package store

import (
	"github.com/tendermint/abci/types"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"

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

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return req.Path == QueryPath
}

func (a *app) Query(state appstate.State, req types.RequestQuery) types.ResponseQuery {
	if !a.AcceptQuery(req) {
		return types.ResponseQuery{
			Code: code.ERROR,
			Log:  "invalid query",
		}
	}

	val := state.Get(req.Data)
	return types.ResponseQuery{
		Value:  val,
		Height: state.Version(),
	}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) types.ResponseCheckTx {
	return types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "store app: unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) types.ResponseDeliverTx {
	return types.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "store app: unknown transaction",
	}
}

package market

import (
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"
)

type app struct {
	state  state.State
	logger log.Logger
}

func NewApp(state state.State, logger log.Logger) (apptypes.Application, error) {
	return &app{state, logger}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return false
}

func (a *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	return tmtypes.ResponseQuery{}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	return tmtypes.ResponseCheckTx{}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	return tmtypes.ResponseDeliverTx{}
}

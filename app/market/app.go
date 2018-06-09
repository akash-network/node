package market

import (
	apptypes "github.com/ovrclk/akash/app/types"
	appstate "github.com/ovrclk/akash/state"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"
)

const (
	Name = "marketplace"
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, logger)}, nil
}

func (a *app) Name() string {
	return Name
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return false
}

func (a *app) Query(state appstate.State, req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	return tmtypes.ResponseQuery{}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	return tmtypes.ResponseCheckTx{}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	return tmtypes.ResponseDeliverTx{}
}

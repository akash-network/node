package market

import (
	apptypes "github.com/ovrclk/akash/app/types"
	appstate "github.com/ovrclk/akash/state"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
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

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return false
}

func (a *app) Query(state appstate.State, req abci_types.RequestQuery) abci_types.ResponseQuery {
	return abci_types.ResponseQuery{}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	return abci_types.ResponseCheckTx{}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	return abci_types.ResponseDeliverTx{}
}

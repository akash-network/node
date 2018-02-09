package app

import (
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/version"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"
)

type app struct {
	tmtypes.BaseApplication

	state state.State
	log   log.Logger
}

var _ tmtypes.Application = &app{}

func Create(state state.State, logger log.Logger) (tmtypes.Application, error) {
	return &app{state: state, log: logger}, nil
}

func (app *app) Info(req tmtypes.RequestInfo) tmtypes.ResponseInfo {
	return tmtypes.ResponseInfo{
		Data:             "{}",
		Version:          version.Version,
		LastBlockHeight:  int64(app.state.Version()),
		LastBlockAppHash: app.state.Hash(),
	}
}

func (app *app) SetOption(req tmtypes.RequestSetOption) tmtypes.ResponseSetOption {
	return tmtypes.ResponseSetOption{Code: tmtypes.CodeTypeOK}
}

func (app *app) Query(tmtypes.RequestQuery) tmtypes.ResponseQuery {
	return tmtypes.ResponseQuery{Code: tmtypes.CodeTypeOK}
}

func (app *app) CheckTx(buf []byte) tmtypes.ResponseCheckTx {

	_, err := txutil.ProcessTx(buf)
	if err != nil {
		app.log.Error("invalid tx: %v", err)
		return tmtypes.ResponseCheckTx{Code: 500}
	}

	return tmtypes.ResponseCheckTx{Code: tmtypes.CodeTypeOK}
}

func (app *app) BeginBlock(req tmtypes.RequestBeginBlock) tmtypes.ResponseBeginBlock {
	return tmtypes.ResponseBeginBlock{}
}

func (app *app) DeliverTx(tx []byte) tmtypes.ResponseDeliverTx {
	return tmtypes.ResponseDeliverTx{Code: tmtypes.CodeTypeOK}
}

func (app *app) EndBlock(req tmtypes.RequestEndBlock) tmtypes.ResponseEndBlock {
	return tmtypes.ResponseEndBlock{}
}

func (app *app) Commit() tmtypes.ResponseCommit {

	version := app.state.Version()

	data, err := app.state.Commit(version + 1)

	if err != nil {
		// todo: list of const response codes
		return tmtypes.ResponseCommit{Code: 404, Data: data, Log: err.Error()}
	}

	return tmtypes.ResponseCommit{Code: tmtypes.CodeTypeOK, Data: data}
}

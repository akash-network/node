package app

import (
	"github.com/ovrclk/photon/app/account"
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/code"
	"github.com/ovrclk/photon/version"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"
)

type app struct {
	tmtypes.BaseApplication

	state state.State

	apps []apptypes.Application

	log log.Logger
}

func Create(state state.State, logger log.Logger) (tmtypes.Application, error) {

	var apps []apptypes.Application

	{
		app, err := account.NewApp(state, logger)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	return &app{state: state, apps: apps, log: logger}, nil
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

func (app *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	for _, app := range app.apps {
		if app.AcceptQuery(req) {
			return app.Query(req)
		}
	}
	return tmtypes.ResponseQuery{Code: code.UNKNOWN_QUERY, Log: "unknown query"}
}

func (app *app) CheckTx(buf []byte) tmtypes.ResponseCheckTx {
	ctx, app_, tx, err := app.appForTx(buf)
	if err != nil {
		return tmtypes.ResponseCheckTx{Code: err.Code(), Log: err.Error()}
	}
	return app_.CheckTx(ctx, tx.Payload.Payload)
}

func (app *app) BeginBlock(req tmtypes.RequestBeginBlock) tmtypes.ResponseBeginBlock {
	return tmtypes.ResponseBeginBlock{}
}

func (app *app) DeliverTx(buf []byte) tmtypes.ResponseDeliverTx {
	ctx, app_, tx, err := app.appForTx(buf)
	if err != nil {
		return tmtypes.ResponseDeliverTx{Code: err.Code(), Log: err.Error()}
	}
	return app_.DeliverTx(ctx, tx.Payload.Payload)
}

func (app *app) EndBlock(req tmtypes.RequestEndBlock) tmtypes.ResponseEndBlock {
	return tmtypes.ResponseEndBlock{}
}

func (app *app) Commit() tmtypes.ResponseCommit {

	version := app.state.Version()

	data, err := app.state.Commit(version + 1)

	if err != nil {
		return tmtypes.ResponseCommit{Data: data, Code: code.ERROR, Log: err.Error()}
	}

	return tmtypes.ResponseCommit{Code: tmtypes.CodeTypeOK, Data: data}
}

func (app *app) appForTx(buf []byte) (
	apptypes.Context, apptypes.Application, *types.Tx, apptypes.Error) {
	tx, err := txutil.ProcessTx(buf)
	if err != nil {
		return nil, nil, nil, apptypes.WrapError(code.ERROR, err)
	}
	ctx := apptypes.NewContext(tx)

	for _, app := range app.apps {
		if app.AcceptTx(ctx, tx.Payload.Payload) {
			return ctx, app, tx, nil
		}
	}

	return nil, nil, nil, apptypes.ErrUnknownTransaction()
}

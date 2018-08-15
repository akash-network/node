package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/akash/app/account"
	"github.com/ovrclk/akash/app/deployment"
	"github.com/ovrclk/akash/app/fulfillment"
	"github.com/ovrclk/akash/app/lease"
	"github.com/ovrclk/akash/app/market"
	"github.com/ovrclk/akash/app/order"
	"github.com/ovrclk/akash/app/provider"
	"github.com/ovrclk/akash/app/store"
	apptypes "github.com/ovrclk/akash/app/types"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	"github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/version"
	tmtypes "github.com/tendermint/abci/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
)

type Application interface {
	tmtypes.Application
	ActivateMarket(market.Actor, *tmtmtypes.EventBus) error

	App(name string) apptypes.Application
}

type app struct {
	tmtypes.BaseApplication

	cacheState  appstate.CacheState
	commitState appstate.CommitState

	apps []apptypes.Application

	mfacilitator market.Driver

	log log.Logger
}

func Create(commitState appstate.CommitState, cacheState appstate.CacheState, logger log.Logger) (Application, error) {

	var apps []apptypes.Application

	{
		app, err := account.NewApp(logger.With("app", account.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := store.NewApp(logger.With("app", store.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := deployment.NewApp(logger.With("app", deployment.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := order.NewApp(logger.With("app", order.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := fulfillment.NewApp(logger.With("app", fulfillment.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := lease.NewApp(logger.With("app", lease.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := provider.NewApp(logger.With("app", provider.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	return &app{commitState: commitState, cacheState: cacheState, apps: apps, log: logger}, nil
}

func (app *app) App(name string) apptypes.Application {
	for _, _app := range app.apps {
		if _app.Name() == name {
			return _app
		}
	}
	return nil
}

func (app *app) ActivateMarket(actor market.Actor, bus *tmtmtypes.EventBus) error {

	if app.mfacilitator != nil {
		return errors.New("market already activated")
	}

	mapp, err := market.NewApp(app.log.With("app", market.Name))
	if err != nil {
		return err
	}

	mfacilitator, err := market.NewDriver(context.Background(), app.log.With("app", "market-facilitator"), actor, bus)
	if err != nil {
		return err
	}

	app.mfacilitator = mfacilitator

	app.apps = append(app.apps, mapp)

	return nil
}

func (app *app) Info(req tmtypes.RequestInfo) tmtypes.ResponseInfo {
	vsn, _ := json.Marshal(version.Get())
	return tmtypes.ResponseInfo{
		Data:             string(vsn),
		Version:          version.Version(),
		LastBlockHeight:  app.commitState.Version(),
		LastBlockAppHash: app.commitState.Hash(),
	}
}

func (app *app) SetOption(req tmtypes.RequestSetOption) tmtypes.ResponseSetOption {
	return tmtypes.ResponseSetOption{Code: tmtypes.CodeTypeOK}
}

func (app *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	app.traceJs("Query", "req", req)
	for _, subapp := range app.apps {
		if subapp.AcceptQuery(req) {
			return subapp.Query(app.commitState, req)
		}
	}
	return tmtypes.ResponseQuery{Code: code.UNKNOWN_QUERY, Log: "unknown query"}
}

func (app *app) checkNonce(state appstate.State, address []byte, nonce uint64) apptypes.Error {
	signer, err_ := state.Account().Get(address)
	if err_ != nil {
		return apptypes.NewError(code.INVALID_TRANSACTION, err_.Error())
	}
	err := apptypes.NewError(code.INVALID_TRANSACTION, "invalid nonce")
	if signer != nil && signer.Nonce >= nonce {
		return err
	}
	return nil
}

func (app *app) CheckTx(buf []byte) tmtypes.ResponseCheckTx {
	ctx, app_, tx, err := app.appForTx(buf)
	if err != nil {
		return tmtypes.ResponseCheckTx{Code: err.Code(), Log: err.Error()}
	}
	app.traceTx("CheckTx", tx)
	err = app.checkNonce(app.commitState, ctx.Signer().Address().Bytes(), tx.Payload.Nonce)
	if err != nil {
		return tmtypes.ResponseCheckTx{Code: err.Code(), Log: err.Error()}
	}
	return app_.CheckTx(app.commitState, ctx, tx.Payload.Payload)
}

func (app *app) DeliverTx(buf []byte) tmtypes.ResponseDeliverTx {
	ctx, app_, tx, err := app.appForTx(buf)
	if err != nil {
		return tmtypes.ResponseDeliverTx{Code: err.Code(), Log: err.Error()}
	}
	app.traceTx("DeliverTx", tx)
	err = app.checkNonce(app.cacheState, ctx.Signer().Address().Bytes(), tx.Payload.Nonce)
	if err != nil {
		return tmtypes.ResponseDeliverTx{Code: err.Code(), Log: err.Error()}
	}
	signer, err_ := app.cacheState.Account().Get(ctx.Signer().Address().Bytes())
	if err_ != nil {
		return tmtypes.ResponseDeliverTx{Code: code.INVALID_TRANSACTION, Log: err_.Error()}
	}

	// XXX: Accouts should be implicitly created when tokens are sent to it
	if signer == nil {
		signer = &types.Account{
			Address: ctx.Signer().Address().Bytes(),
			Balance: 0,
			Nonce:   0,
		}
	}

	if err_ := app.cacheState.Account().Save(signer); err_ != nil {
		return tmtypes.ResponseDeliverTx{Code: code.INVALID_TRANSACTION, Log: err_.Error()}
	}

	resp := app_.DeliverTx(app.cacheState, ctx, tx.Payload.Payload)

	// set new account nonce
	if resp.IsOK() {
		signer, err_ = app.cacheState.Account().Get(ctx.Signer().Address().Bytes())
		if err_ != nil {
			return tmtypes.ResponseDeliverTx{Code: code.INVALID_TRANSACTION, Log: err_.Error()}
		}
		signer.Nonce = tx.Payload.Nonce
		if err_ := app.cacheState.Account().Save(signer); err_ != nil {
			return tmtypes.ResponseDeliverTx{Code: code.INVALID_TRANSACTION, Log: err_.Error()}
		}
	}

	return resp
}

func (app *app) BeginBlock(req tmtypes.RequestBeginBlock) tmtypes.ResponseBeginBlock {
	app.trace("BeginBlock", "tmhash", util.X(req.Hash))

	if app.mfacilitator != nil {
		app.mfacilitator.OnBeginBlock(req)
	}

	return tmtypes.ResponseBeginBlock{}
}

func (app *app) EndBlock(req tmtypes.RequestEndBlock) tmtypes.ResponseEndBlock {
	app.trace("EndBlock")
	return tmtypes.ResponseEndBlock{}
}

func (app *app) Commit() tmtypes.ResponseCommit {
	app.trace("Commit")

	if err := lease.ProcessLeases(app.cacheState); err != nil {
		app.log.Error("processing leases", "error", err)
	}

	if err := app.cacheState.Write(); err != nil {
		panic("error when writing to cache")
	}

	data, _, err := app.commitState.Commit()
	if err != nil {
		return tmtypes.ResponseCommit{Data: data}
	}

	if app.mfacilitator != nil {
		err := app.mfacilitator.OnCommit(app.commitState)
		if err != nil {
			app.log.Error("error in facilitator.OnCommit", err.Error())
		}
	}

	return tmtypes.ResponseCommit{Data: data}
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

func (app *app) traceJs(meth string, name string, obj interface{}) {
	js, err := json.Marshal(obj)
	if err != nil {
		app.traceLog().Error(meth, "trace-error", err)
		return
	}
	app.traceLog().Debug(meth, name, string(js))
}

func (app *app) traceTx(meth string, obj *types.Tx) {
	m := jsonpb.Marshaler{}
	js, err := m.MarshalToString(obj)
	if err != nil {
		app.traceLog().Error(meth, "trace-error", err)
		return
	}
	app.traceLog().Debug(meth, "tx", js)
}

func (app *app) trace(meth string, keyvals ...interface{}) {
	app.traceLog().Debug(meth, keyvals...)
}

func (app *app) traceLog() log.Logger {
	return app.log.With("height", app.commitState.Version(), "hash", util.X(app.commitState.Hash()), "logtype", "trace")
}

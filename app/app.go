package app

import (
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/photon/app/account"
	"github.com/ovrclk/photon/app/deployment"
	"github.com/ovrclk/photon/app/fulfillmentorder"
	"github.com/ovrclk/photon/app/lease"
	"github.com/ovrclk/photon/app/market"
	"github.com/ovrclk/photon/app/order"
	"github.com/ovrclk/photon/app/provider"
	"github.com/ovrclk/photon/app/store"
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/code"
	"github.com/ovrclk/photon/version"
	tmtypes "github.com/tendermint/abci/types"
	tmtmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tmlibs/log"
)

type Application interface {
	tmtypes.Application
	ActivateMarket(market.Actor, *tmtmtypes.EventBus) error
}

type app struct {
	tmtypes.BaseApplication

	state state.State

	apps []apptypes.Application

	mfacilitator market.Driver

	log log.Logger
}

func Create(state state.State, logger log.Logger) (Application, error) {

	var apps []apptypes.Application

	{
		app, err := account.NewApp(state, logger.With("app", account.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := store.NewApp(state, logger.With("app", store.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := deployment.NewApp(state, logger.With("app", deployment.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := order.NewApp(state, logger.With("app", order.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := fulfillmentorder.NewApp(state, logger.With("app", fulfillmentorder.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := lease.NewApp(state, logger.With("app", lease.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := provider.NewApp(state, logger.With("app", provider.Name))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	return &app{state: state, apps: apps, log: logger}, nil
}

func (app *app) ActivateMarket(actor market.Actor, bus *tmtmtypes.EventBus) error {

	if app.mfacilitator != nil {
		return errors.New("market already activated")
	}

	mapp, err := market.NewApp(app.state, app.log.With("app", market.Name))
	if err != nil {
		return err
	}

	mfacilitator, err := market.NewDriver(app.log.With("app", "market-facilitator"), actor, bus)
	if err != nil {
		return err
	}

	app.mfacilitator = mfacilitator

	app.apps = append(app.apps, mapp)

	return nil
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
	app.traceJs("Query", "req", req)
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
	app.traceTx("CheckTx", tx)
	return app_.CheckTx(ctx, tx.Payload.Payload)
}

func (app *app) DeliverTx(buf []byte) tmtypes.ResponseDeliverTx {
	ctx, app_, tx, err := app.appForTx(buf)
	if err != nil {
		return tmtypes.ResponseDeliverTx{Code: err.Code(), Log: err.Error()}
	}

	signer, err_ := app.state.Account().Get(ctx.Signer().Address())
	if err_ != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err_.Error(),
		}
	}

	if signer == nil {
		// return tmtypes.ResponseDeliverTx{
		// 	Code: code.INVALID_TRANSACTION,
		// 	Log:  "unknown signer account",
		// }
		signer = &types.Account{
			Address: ctx.Signer().Address(),
			Balance: 0,
			Nonce:   0,
		}
	}

	if signer.Nonce >= tx.Payload.Nonce {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "invalid nonce",
		}
	}

	signer.Nonce = tx.Payload.Nonce

	if err_ := app.state.Account().Save(signer); err_ != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err_.Error(),
		}
	}

	app.traceTx("DeliverTx", tx)
	return app_.DeliverTx(ctx, tx.Payload.Payload)
}

func (app *app) BeginBlock(req tmtypes.RequestBeginBlock) tmtypes.ResponseBeginBlock {
	app.trace("BeginBlock", "tmhash", hex.EncodeToString(req.Hash))

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

	data, _, err := app.state.Commit()

	if err != nil {
		return tmtypes.ResponseCommit{Data: data}
	}

	if app.mfacilitator != nil {
		app.mfacilitator.OnCommit(app.state)
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
	return app.log.With("height", app.state.Version(), "hash", hex.EncodeToString(app.state.Hash()), "logtype", "trace")
}

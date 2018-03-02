package app

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/photon/app/account"
	"github.com/ovrclk/photon/app/datacenter"
	"github.com/ovrclk/photon/app/deployment"
	"github.com/ovrclk/photon/app/deploymentOrder"
	"github.com/ovrclk/photon/app/store"
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
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
		app, err := account.NewApp(state, logger.With("app", "account"))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := store.NewApp(state, logger.With("app", "store"))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := deployment.NewApp(state, logger.With("app", "deployment"))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := deploymentOrder.NewApp(state, logger.With("app", "deploymentorder"))
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	{
		app, err := datacenter.NewApp(state, logger.With("app", "datacenter"))
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
	app.trace("BeginBlock")
	return tmtypes.ResponseBeginBlock{}
}

func doHash(address []byte, nonce uint64) []byte {
	nbytes := make([]byte, 10)
	binary.LittleEndian.PutUint64(nbytes, nonce)
	data := append(address, nbytes...)
	hash32 := sha256.Sum256(data)
	return hash32[:32]
}

func (app *app) createDeploymentOrders() {
	// create deploymentOrders for deployments without matching active deployment orders

	// get all deployments
	// todo: would storing deployments by state (open, close, etc.) cause a significant over performance increase?
	start := new(base.Bytes)
	start.DecodeString("")
	end := new(base.Bytes)
	end.DecodeString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
	limit := math.MaxInt64
	depo := &types.DeploymentOrder{}
	_, deps, _, _ := app.state.Deployment().GetRangeWithProof(*start, *end, limit)

	// create deploymentOrders for deployments with open state
	for _, deployment := range deps.Deployments {
		if deployment.State == types.Deployment_OPEN {
			// create deployment order
			// generate new unique address for deployment order
			depo.Address = doHash(deployment.Address, deployment.Nonce)
			depo.From = deployment.From
			depo.Groups = deployment.Groups
			depo.State = types.DeploymentOrder_OPEN

			err := app.state.DeploymentOrder().Save(depo)
			if err != nil {
				app.trace("ERROR: deploymentOrder save failed")
			}
			// change deployment state to Deployment_ORDERED
			deployment.State = types.Deployment_ORDERED
			deployment.Nonce += 1
			err = app.state.Deployment().Save(&deployment)
			if err != nil {
				app.trace("ERROR: deployment save failed")
			}
		}
	}
}

func (app *app) EndBlock(req tmtypes.RequestEndBlock) tmtypes.ResponseEndBlock {
	app.trace("EndBlock")
	app.createDeploymentOrders()
	return tmtypes.ResponseEndBlock{}
}

func (app *app) Commit() tmtypes.ResponseCommit {
	app.trace("Commit")

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

func (app *app) trace(meth string) {
	app.traceLog().Debug(meth)
}

func (app *app) traceLog() log.Logger {
	return app.log.With("height", app.state.Version(), "hash", hex.EncodeToString(app.state.Hash()), "logtype", "trace")
}

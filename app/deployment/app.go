package deployment

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/types/code"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/go-wire/data"
	"github.com/tendermint/tmlibs/log"
)

const (
	Name = apptypes.TagAppDeployment
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(state state.State, logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, state, logger)}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), state.DeploymentPath) || strings.HasPrefix(req.GetPath(), state.DeploymentGroupPath)
}

func (a *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {

	if !a.AcceptQuery(req) {
		return tmtypes.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
	}

	// todo: need abtraction for multiple query types per app
	if strings.HasPrefix(req.GetPath(), state.DeploymentGroupPath) {
		id := strings.TrimPrefix(req.Path, state.DeploymentPath)
		key := new(base.Bytes)
		if err := key.DecodeString(id); err != nil {
			return tmtypes.ResponseQuery{
				Code: code.ERROR,
				Log:  err.Error(),
			}
		}
		return a.doDeploymentGroupQuery(*key)
	}

	// todo: abstractiion: all queries should have this
	id := strings.TrimPrefix(req.Path, state.DeploymentPath)
	key := new(base.Bytes)
	if err := key.DecodeString(id); err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	// id is empty string, get full range
	if len(id) == 0 {
		return a.doRangeQuery(*key)
	}
	return a.doQuery(*key)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return a.doCheckTx(ctx, tx.TxCreateDeployment)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return a.doDeliverTx(ctx, tx.TxCreateDeployment)
	}
	return tmtypes.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) doQuery(key base.Bytes) tmtypes.ResponseQuery {

	dep, err := a.State().Deployment().Get(key)

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if dep == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment %x not found", key),
		}
	}

	bytes, err := proto.Marshal(dep)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Key:    data.Bytes(a.State().Deployment().KeyFor(key)),
		Value:  bytes,
		Height: a.State().Version(),
	}
}

func (a *app) doRangeQuery(key base.Bytes) tmtypes.ResponseQuery {
	deps, err := a.State().Deployment().GetMaxRange()
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	bytes, err := proto.Marshal(deps)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Key:    data.Bytes(state.DeploymentPath),
		Value:  bytes,
		Height: a.State().Version(),
	}
}

func (a *app) doDeploymentGroupQuery(key base.Bytes) tmtypes.ResponseQuery {

	dep, err := a.State().DeploymentGroup().GetByKey(key)

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if dep == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment group %x not found", key),
		}
	}

	bytes, err := proto.Marshal(dep)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Key:    data.Bytes(a.State().DeploymentGroup().KeyFor(key)),
		Value:  bytes,
		Height: a.State().Version(),
	}
}

// todo: break each type of check out into a named global exported funtion for all trasaction types to utilize
func (a *app) doCheckTx(ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseCheckTx {

	if !bytes.Equal(ctx.Signer().Address(), tx.Deployment.Tenant) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by sending address",
		}
	}

	acct, err := a.State().Account().Get(tx.Deployment.Tenant)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if acct == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "unknown source account",
		}
	}

	return tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverTx(ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deployment := tx.Deployment

	seq := a.State().Deployment().SequenceFor(deployment.Address)

	for _, group := range deployment.Groups {
		group.Deployment = deployment.Address
		group.Seq = seq.Advance()
		a.State().DeploymentGroup().Save(&group)
	}

	if err := a.State().Deployment().Save(deployment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeDeployment),
	}
}

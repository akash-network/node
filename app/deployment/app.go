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
		id := strings.TrimPrefix(req.Path, state.DeploymentGroupPath)
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
	case *types.TxPayload_TxCloseDeployment:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return a.doCheckCreateTx(ctx, tx.TxCreateDeployment)
	case *types.TxPayload_TxCloseDeployment:
		return a.doCheckCloseTx(ctx, tx.TxCloseDeployment)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return a.doDeliverCreateTx(ctx, tx.TxCreateDeployment)
	case *types.TxPayload_TxCloseDeployment:
		return a.doDeliverCloseTx(ctx, tx.TxCloseDeployment)
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
		Value:  bytes,
		Height: a.State().Version(),
	}
}

func (a *app) doCheckCreateTx(ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseCheckTx {

	if !bytes.Equal(ctx.Signer().Address(), tx.Tenant) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by sending address",
		}
	}

	if len(tx.Groups) == 0 {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "No groups in deployment",
		}
	}

	acct, err := a.State().Account().Get(tx.Tenant)
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

	if acct.Nonce >= tx.Nonce {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "invalid nonce",
		}
	}

	return tmtypes.ResponseCheckTx{}
}

func (a *app) doCheckCloseTx(ctx apptypes.Context, tx *types.TxCloseDeployment) tmtypes.ResponseCheckTx {
	deployment, err := a.State().Deployment().Get(tx.Deployment)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if deployment == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment not found",
		}
	}

	switch tx.Reason {
	case types.TxCloseDeployment_INSUFFICIENT:
		// XXX: signer must be block's facilitator
	case types.TxCloseDeployment_TENANT_CLOSE:
		if !bytes.Equal(ctx.Signer().Address(), deployment.Tenant) {
			return tmtypes.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "Not signed by tenant address",
			}
		}
	default:
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Invalid reason",
		}
	}

	if deployment.State != types.Deployment_ACTIVE {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment not active",
		}
	}

	return tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverCreateTx(ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckCreateTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deployment := &types.Deployment{
		Address: state.DeploymentAddress(tx.Tenant, tx.Nonce),
		Tenant:  tx.Tenant,
		State:   types.Deployment_ACTIVE,
	}

	seq := a.State().Deployment().SequenceFor(deployment.Address)

	groups := tx.Groups

	for _, group := range groups {
		g := &types.DeploymentGroup{
			Deployment:   deployment.Address,
			Seq:          seq.Advance(),
			State:        types.DeploymentGroup_OPEN,
			Requirements: group.Requirements,
			Resources:    group.Resources,
			OrderTTL:     tx.OrderTTL,
		}
		a.State().DeploymentGroup().Save(g)
	}

	if err := a.State().Deployment().Save(deployment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateDeployment),
		Data: deployment.Address,
	}
}

func (a *app) doDeliverCloseTx(ctx apptypes.Context, tx *types.TxCloseDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckCloseTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deployment, err := a.State().Deployment().Get(tx.Deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if deployment == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment not found",
		}
	}

	deployment.State = types.Deployment_CLOSED
	err = a.State().Deployment().Save(deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	groups, err := a.State().DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if groups == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment groups",
		}
	}

	leases, err := a.State().Lease().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, group := range groups {
		group.State = types.DeploymentGroup_CLOSED
		err = a.State().DeploymentGroup().Save(group)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
	}

	orders, err := a.State().Order().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, order := range orders {
		order.State = types.Order_CLOSED
		err = a.State().Order().Save(order)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
	}

	fulfillments, err := a.State().Fulfillment().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, fulfillment := range fulfillments {
		fulfillment.State = types.Fulfillment_CLOSED
		err = a.State().Fulfillment().Save(fulfillment)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
	}

	if leases != nil {
		for _, lease := range leases {
			lease.State = types.Lease_CLOSED
			err = a.State().Lease().Save(lease)
			if err != nil {
				return tmtypes.ResponseDeliverTx{
					Code: code.INVALID_TRANSACTION,
					Log:  err.Error(),
				}
			}
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCloseDeployment),
	}
}

package deployment

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	"github.com/ovrclk/akash/validation"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/tmlibs/log"
)

const (
	Name = apptypes.TagAppDeployment
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, logger)}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.DeploymentPath) || strings.HasPrefix(req.GetPath(), appstate.DeploymentGroupPath)
}

func (a *app) Query(state appstate.State, req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	if !a.AcceptQuery(req) {
		return tmtypes.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
	}

	// TODO: Partial Key Parsing

	if strings.HasPrefix(req.GetPath(), appstate.DeploymentGroupPath) {
		id := strings.TrimPrefix(req.Path, appstate.DeploymentGroupPath)

		if len(id) == 0 {
			return a.doDeploymentGroupRangeQuery(state)
		}

		key, err := keys.ParseGroupPath(id)
		if err != nil {
			return tmtypes.ResponseQuery{
				Code: code.ERROR,
				Log:  err.Error(),
			}
		}
		return a.doDeploymentGroupQuery(state, key)
	}

	id := strings.TrimPrefix(req.Path, appstate.DeploymentPath)
	if len(id) == 0 {
		return a.doRangeQuery(state, req.Data)
	}

	key, err := keys.ParseDeploymentPath(id)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return a.doQuery(state, key)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return true
	case *types.TxPayload_TxUpdateDeployment:
		return true
	case *types.TxPayload_TxCloseDeployment:
		return true
	}
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return a.doCheckCreateTx(state, ctx, tx.TxCreateDeployment)
	case *types.TxPayload_TxUpdateDeployment:
		return a.doCheckUpdateTx(state, ctx, tx.TxUpdateDeployment)
	case *types.TxPayload_TxCloseDeployment:
		return a.doCheckCloseTx(state, ctx, tx.TxCloseDeployment)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeployment:
		return a.doDeliverCreateTx(state, ctx, tx.TxCreateDeployment)
	case *types.TxPayload_TxUpdateDeployment:
		return a.doDeliverUpdateTx(state, ctx, tx.TxUpdateDeployment)
	case *types.TxPayload_TxCloseDeployment:
		return a.doDeliverCloseTx(state, ctx, tx.TxCloseDeployment)
	}
	return tmtypes.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) doQuery(state appstate.State, key keys.Deployment) tmtypes.ResponseQuery {

	dep, err := state.Deployment().Get(key.ID())

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if dep == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment %v not found", key),
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
		Height: state.Version(),
	}
}

func (a *app) doRangeQuery(state appstate.State, tenant []byte) tmtypes.ResponseQuery {
	deps, err := state.Deployment().GetMaxRange()
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	tenantDeps := []types.Deployment{}
	for _, deployment := range deps.Items {
		if len(tenant) == 0 || bytes.Equal(deployment.Tenant, tenant) {
			tenantDeps = append(tenantDeps, deployment)
		}
	}

	deps.Items = tenantDeps
	bytes, err := proto.Marshal(deps)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) doDeploymentGroupRangeQuery(state appstate.State) tmtypes.ResponseQuery {
	objs, err := state.DeploymentGroup().All()
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	bytes, err := proto.Marshal(&types.DeploymentGroups{
		Items: objs,
	})
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) doDeploymentGroupQuery(state appstate.State, key keys.DeploymentGroup) tmtypes.ResponseQuery {

	dep, err := state.DeploymentGroup().Get(key.ID())

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if dep == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment group %v not found", key.Path()),
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
		Height: state.Version(),
	}
}

func (a *app) doCheckCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseCheckTx {

	if !bytes.Equal(ctx.Signer().Address(), tx.Tenant) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by sending address",
		}
	}

	if err := validation.ValidateGroupSpecs(tx.Groups); err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, group := range tx.Groups {
		for _, resource := range group.Resources {
			if resource.Price == 0 {
				return tmtypes.ResponseCheckTx{
					Code: code.INVALID_TRANSACTION,
					Log:  "Resources must have a non-zero price",
				}
			}
		}
	}

	acct, err := state.Account().Get(tx.Tenant)
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

func (a *app) doCheckUpdateTx(
	state appstate.State,
	ctx apptypes.Context,
	tx *types.TxUpdateDeployment) tmtypes.ResponseCheckTx {

	if len(tx.Version) == 0 {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "invalid version: empty",
		}
	}

	deployment, err := state.Deployment().Get(tx.Deployment)
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

	if !bytes.Equal(ctx.Signer().Address(), deployment.Tenant) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment not owned by signer",
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

func (a *app) doCheckCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseDeployment) tmtypes.ResponseCheckTx {
	deployment, err := state.Deployment().Get(tx.Deployment)
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

func (a *app) doDeliverCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckCreateTx(state, ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deployment := &types.Deployment{
		Address: appstate.DeploymentAddress(tx.Tenant, tx.Nonce),
		Tenant:  tx.Tenant,
		State:   types.Deployment_ACTIVE,
		Version: tx.Version,
	}

	seq := state.Deployment().SequenceFor(deployment.Address)

	groups := tx.Groups

	for _, group := range groups {
		g := &types.DeploymentGroup{
			DeploymentGroupID: types.DeploymentGroupID{
				Deployment: deployment.Address,
				Seq:        seq.Advance(),
			},
			Name:         group.Name,
			State:        types.DeploymentGroup_OPEN,
			Requirements: group.Requirements,
			Resources:    group.Resources,
			OrderTTL:     tx.OrderTTL,
		}
		err := state.DeploymentGroup().Save(g)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "error saving deployment group" + err.Error(),
			}
		}
	}

	if err := state.Deployment().Save(deployment); err != nil {
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

func (a *app) doDeliverUpdateTx(
	state appstate.State,
	ctx apptypes.Context,
	tx *types.TxUpdateDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckUpdateTx(state, ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deployment, err := state.Deployment().Get(tx.Deployment)
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

	deployment.Version = tx.Version

	if err := state.Deployment().Save(deployment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeUpdateDeployment),
		Data: deployment.Address,
	}
}

func (a *app) doDeliverCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckCloseTx(state, ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deployment, err := state.Deployment().Get(tx.Deployment)
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
	err = state.Deployment().Save(deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	groups, err := state.DeploymentGroup().ForDeployment(deployment.Address)
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

	leases, err := state.Lease().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, group := range groups {
		group.State = types.DeploymentGroup_CLOSED
		err = state.DeploymentGroup().Save(group)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
	}

	orders, err := state.Order().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, order := range orders {
		order.State = types.Order_CLOSED
		err = state.Order().Save(order)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
	}

	fulfillments, err := state.Fulfillment().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	for _, fulfillment := range fulfillments {
		fulfillment.State = types.Fulfillment_CLOSED
		err = state.Fulfillment().Save(fulfillment)
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
			err = state.Lease().Save(lease)
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

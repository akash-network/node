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
	case *types.TxPayload_TxDeploymentClosed:
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
	case *types.TxPayload_TxDeploymentClosed:
		return a.doCheckClosedTx(ctx, tx.TxDeploymentClosed)
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
	case *types.TxPayload_TxDeploymentClosed:
		return a.doDeliverClosedTx(ctx, tx.TxDeploymentClosed)
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

func (a *app) doCheckCreateTx(ctx apptypes.Context, tx *types.TxCreateDeployment) tmtypes.ResponseCheckTx {

	if !bytes.Equal(ctx.Signer().Address(), tx.Deployment.Tenant) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by sending address",
		}
	}

	if len(tx.Groups.GetItems()) == 0 {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "No groups in deployment",
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

	if !bytes.Equal(ctx.Signer().Address(), deployment.Tenant) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by tenant address",
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

func (a *app) doCheckClosedTx(ctx apptypes.Context, tx *types.TxDeploymentClosed) tmtypes.ResponseCheckTx {

	// todo: check signed by block facilitator

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

	groups, err := a.State().DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if deployment == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment groups",
		}
	}

	// check each object related to the deployment is also closing state
	for _, group := range groups {
		// begin for each group
		if group.State != types.DeploymentGroup_CLOSING {
			return tmtypes.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "Deployment group not closed",
			}
		}

		orders, err := a.State().Order().ForGroup(group)
		if err != nil {
			return tmtypes.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}

		for _, order := range orders {
			// begin for each order
			if order.State != types.Order_CLOSING {
				return tmtypes.ResponseCheckTx{
					Code: code.INVALID_TRANSACTION,
					Log:  "Order not closed",
				}
			}

			fulfillments, err := a.State().Fulfillment().ForOrder(order)
			if err != nil {
				return tmtypes.ResponseCheckTx{
					Code: code.INVALID_TRANSACTION,
					Log:  err.Error(),
				}
			}

			for _, fulfillment := range fulfillments {
				// begin for each fulfillment
				if fulfillment.State != types.Fulfillment_CLOSING {
					return tmtypes.ResponseCheckTx{
						Code: code.INVALID_TRANSACTION,
						Log:  "Fulfillment not closed",
					}
				}

				lease, err := a.State().Lease().Get(deployment.Address, group.Seq, order.Order, fulfillment.Provider)
				if err != nil {
					return tmtypes.ResponseCheckTx{
						Code: code.INVALID_TRANSACTION,
						Log:  err.Error(),
					}
				}
				if lease != nil && lease.State != types.Lease_CLOSING {
					return tmtypes.ResponseCheckTx{
						Code: code.INVALID_TRANSACTION,
						Log:  "Lease not closed",
					}
				}
				// end for each fulfillment
			}
			// end for each order
		}
		// end for each group
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

	deployment := tx.Deployment

	seq := a.State().Deployment().SequenceFor(deployment.Address)

	groups := tx.Groups.GetItems()

	for _, group := range groups {
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
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateDeployment),
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

	groups, err := a.State().DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if deployment == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment groups",
		}
	}

	deployment.State = types.Deployment_CLOSING

	for i, group := range groups {
		// begin for each group
		if group.State != types.DeploymentGroup_CLOSING {
			group.State = types.DeploymentGroup_CLOSING
			groups[i] = group
		}

		orders, err := a.State().Order().ForGroup(group)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}

		for _, order := range orders {
			// begin for each otder
			if order.State != types.Order_CLOSING {
				order.State = types.Order_CLOSING
			}

			fulfillments, err := a.State().Fulfillment().ForOrder(order)
			if err != nil {
				return tmtypes.ResponseDeliverTx{
					Code: code.INVALID_TRANSACTION,
					Log:  err.Error(),
				}
			}

			for _, fulfillment := range fulfillments {
				// begin for each fulfillment
				if fulfillment.State != types.Fulfillment_CLOSING {
					fulfillment.State = types.Fulfillment_CLOSING
				}

				lease, err := a.State().Lease().Get(deployment.Address, group.Seq, order.Order, fulfillment.Provider)
				if err != nil {
					return tmtypes.ResponseDeliverTx{
						Code: code.INVALID_TRANSACTION,
						Log:  err.Error(),
					}
				}
				if lease != nil && lease.State == types.Lease_ACTIVE {
					lease.State = types.Lease_CLOSING
					err = a.State().Lease().Save(lease)
					if err != nil {
						return tmtypes.ResponseDeliverTx{
							Code: code.INVALID_TRANSACTION,
							Log:  err.Error(),
						}
					}
				}

				err = a.State().Fulfillment().Save(fulfillment)
				if err != nil {
					return tmtypes.ResponseDeliverTx{
						Code: code.INVALID_TRANSACTION,
						Log:  err.Error(),
					}
				}
				// end for each fulfillment
			}

			println("\n\n\n\n\nsavign order", order.GoString(), "\n\n\n\n\n")

			err = a.State().Order().Save(order)
			if err != nil {
				return tmtypes.ResponseDeliverTx{
					Code: code.INVALID_TRANSACTION,
					Log:  err.Error(),
				}
			}
			// end for each order
		}
		err = a.State().DeploymentGroup().Save(group)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
		// end for each group
	}

	err = a.State().Deployment().Save(deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCloseDeployment),
	}
}

func (a *app) doDeliverClosedTx(ctx apptypes.Context, tx *types.TxDeploymentClosed) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckClosedTx(ctx, tx)
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

	groups, err := a.State().DeploymentGroup().ForDeployment(deployment.Address)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if deployment == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment groups",
		}
	}

	for i, group := range groups {
		// begin for each group
		if group.State != types.DeploymentGroup_CLOSED {
			group.State = types.DeploymentGroup_CLOSED
			groups[i] = group
		}

		orders, err := a.State().Order().ForGroup(group)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}

		for _, order := range orders {
			// begin for each otder
			if order.State != types.Order_CLOSED {
				order.State = types.Order_CLOSED
			}

			fulfillments, err := a.State().Fulfillment().ForOrder(order)
			if err != nil {
				return tmtypes.ResponseDeliverTx{
					Code: code.INVALID_TRANSACTION,
					Log:  err.Error(),
				}
			}

			for _, fulfillment := range fulfillments {
				// begin for each fulfillment
				if fulfillment.State != types.Fulfillment_CLOSED {
					fulfillment.State = types.Fulfillment_CLOSED
				}

				lease, err := a.State().Lease().Get(deployment.Address, group.Seq, order.Order, fulfillment.Provider)
				if err != nil {
					return tmtypes.ResponseDeliverTx{
						Code: code.INVALID_TRANSACTION,
						Log:  err.Error(),
					}
				}
				if lease != nil && lease.State != types.Lease_CLOSED {
					lease.State = types.Lease_CLOSED
					err = a.State().Lease().Save(lease)
					if err != nil {
						return tmtypes.ResponseDeliverTx{
							Code: code.INVALID_TRANSACTION,
							Log:  err.Error(),
						}
					}
				}

				err = a.State().Fulfillment().Save(fulfillment)
				if err != nil {
					return tmtypes.ResponseDeliverTx{
						Code: code.INVALID_TRANSACTION,
						Log:  err.Error(),
					}
				}
				// end for each fulfillment
			}

			err = a.State().Order().Save(order)
			if err != nil {
				return tmtypes.ResponseDeliverTx{
					Code: code.INVALID_TRANSACTION,
					Log:  err.Error(),
				}
			}
			// end for each order
		}
		err = a.State().DeploymentGroup().Save(group)
		if err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
		// end for each group
	}

	err = a.State().Deployment().Save(deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	err = a.State().Deployment().Save(deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeDeploymentClosed),
	}
}

package lease

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
	abci_types "github.com/tendermint/tendermint/abci/types"
	tmcommon "github.com/tendermint/tendermint/libs/common"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	Name = apptypes.TagAppLease
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(log log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, log)}, nil
}

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.LeasePath)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCloseLease:
		return true
	}
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCloseLease:
		resp, _ := a.doCheckCloseTx(state, ctx, tx.TxCloseLease)
		return resp
	}
	return abci_types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCloseLease:
		return a.doDeliverCloseTx(state, ctx, tx.TxCloseLease)
	}
	return abci_types.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) Query(state appstate.State, req abci_types.RequestQuery) abci_types.ResponseQuery {
	if !a.AcceptQuery(req) {
		return abci_types.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
	}

	// TODO: Partial Key Parsing
	id := strings.TrimPrefix(req.Path, appstate.LeasePath)

	if len(id) == 0 {
		return a.doRangeQuery(state, req.Data)
	}

	{
		key, err := keys.ParseLeasePath(id)
		if err == nil {
			return a.doQuery(state, key)
		}
	}

	key, err := keys.ParseDeploymentPath(id)
	if err == nil {
		return a.doDeploymentQuery(state, key)
	}

	return abci_types.ResponseQuery{
		Code: code.ERROR,
		Log:  err.Error(),
	}
}

func (a *app) doCheckCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseLease) (abci_types.ResponseCheckTx, *types.Lease) {

	// lookup provider
	lease, err := state.Lease().Get(tx.LeaseID)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if lease == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "lease not found",
		}, nil
	}

	if lease.State != types.Lease_ACTIVE {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "lease not active",
		}, nil
	}

	return abci_types.ResponseCheckTx{}, lease
}

func (a *app) doDeliverCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseLease) abci_types.ResponseDeliverTx {
	cresp, lease := a.doCheckCloseTx(state, ctx, tx)
	if !cresp.IsOK() {
		return abci_types.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	group, err := state.DeploymentGroup().Get(lease.GroupID())
	if err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if group == nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "group not found",
		}
	}

	order, err := state.Order().Get(lease.OrderID())
	if err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if order == nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not found",
		}
	}

	order.State = types.Order_CLOSED
	if err := state.Order().Save(order); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	group.State = types.DeploymentGroup_OPEN
	if err := state.DeploymentGroup().Save(group); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	lease.State = types.Lease_CLOSED
	if err := state.Lease().Save(lease); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseDeliverTx{
		Events: apptypes.Events(a.Name(), apptypes.TxTypeCloseLease,
			tmcommon.KVPair{Key: []byte(apptypes.TagNameLease), Value: keys.LeaseID(lease.LeaseID).Bytes()}),
	}
}

func (a *app) doQuery(state appstate.State, key keys.Lease) abci_types.ResponseQuery {
	lease, err := state.Lease().Get(key.ID())

	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if lease == nil {
		return abci_types.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("lease %v not found", key.Path()),
		}
	}

	bytes, err := proto.Marshal(lease)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) doRangeQuery(state appstate.State, tenant []byte) abci_types.ResponseQuery {
	leases, err := state.Lease().All()
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	items := []*types.Lease{}
	for _, lease := range leases {
		deployment, err := state.Deployment().Get(lease.Deployment)
		if err != nil {
			a.Log().Error("deployment doesn't exist for lease")
		}
		if len(tenant) == 0 || bytes.Equal(deployment.Tenant, tenant) {
			items = append(items, lease)
		}
	}

	coll := &types.Leases{Items: items}

	bytes, err := proto.Marshal(coll)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

func (a *app) doDeploymentQuery(state appstate.State, key keys.Deployment) abci_types.ResponseQuery {
	items, err := state.Lease().ForDeployment(key.Bytes())
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	coll := &types.Leases{Items: items}

	bytes, err := proto.Marshal(coll)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: state.Version(),
	}
}

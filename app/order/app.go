package order

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	abci_types "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
)

const (
	Name = apptypes.TagAppOrder
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, logger)}, nil
}

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.OrderPath)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateOrder:
		return true
	}
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateOrder:
		return a.doCheckCreateTx(state, ctx, tx.TxCreateOrder)
	}
	return abci_types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateOrder:
		return a.doDeliverCreateTx(state, ctx, tx.TxCreateOrder)
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
	id := strings.TrimPrefix(req.Path, appstate.OrderPath)
	if len(id) == 0 {
		return a.doRangeQuery(state)
	}

	key, err := keys.ParseOrderPath(id)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	return a.doQuery(state, key)
}

func (a *app) doQuery(state appstate.State, key keys.Order) abci_types.ResponseQuery {

	depo, err := state.Order().Get(key.ID())

	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if depo == nil {
		return abci_types.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("order %v not found", key.Path()),
		}
	}

	bytes, err := proto.Marshal(depo)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseQuery{
		Value:  bytes,
		Height: int64(state.Version()),
	}
}

func (a *app) doRangeQuery(state appstate.State) abci_types.ResponseQuery {
	items, err := state.Order().All()
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	coll := &types.Orders{Items: items}

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

func (a *app) doCheckCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateOrder) abci_types.ResponseCheckTx {

	// todo: ensure signed by last block creator / valid market facilitator

	// ensure order provided
	if tx.Deployment == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "No deployment specified",
		}
	}

	// ensure deployment exists
	deployment, err := state.Deployment().Get(tx.Deployment)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if deployment == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment not found",
		}
	}

	// ensure deployment in correct state
	if deployment.GetState() != types.Deployment_ACTIVE {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment not in active state",
		}
	}

	// ensure deployment group exists
	group, err := state.DeploymentGroup().Get(tx.GroupID())
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if group == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Group not found",
		}
	}

	// ensure deployment group in correct state
	if group.GetState() != types.DeploymentGroup_OPEN {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Group not in open state",
		}
	}

	// ensure no other open orders
	others, err := state.Order().ForGroup(group.DeploymentGroupID)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	for _, other := range others {
		if other.GetState() == types.Order_OPEN {
			return abci_types.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "Order already exists for group",
			}
		}
	}

	return abci_types.ResponseCheckTx{}
}

func (a *app) doDeliverCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateOrder) abci_types.ResponseDeliverTx {

	cresp := a.doCheckCreateTx(state, ctx, tx)
	if !cresp.IsOK() {
		return abci_types.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	oseq := state.Deployment().SequenceFor(tx.Deployment)
	oseq.Advance()

	order := &types.Order{
		OrderID: tx.OrderID,
		EndAt:   tx.EndAt,
		State:   types.Order_OPEN,
	}

	// order.Order = oseq.Advance()
	order.State = types.Order_OPEN

	if err := state.Order().Save(order); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateOrder),
	}
}

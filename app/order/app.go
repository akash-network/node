package order

import (
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
	Name = apptypes.TagAppOrder
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(state state.State, logger log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, state, logger)}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), state.OrderPath)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateOrder:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateOrder:
		return a.doCheckCreateTx(ctx, tx.TxCreateOrder)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateOrder:
		return a.doDeliverCreateTx(ctx, tx.TxCreateOrder)
	}
	return tmtypes.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	if !a.AcceptQuery(req) {
		return tmtypes.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
	}

	// todo: abstractiion: all queries should have this
	id := strings.TrimPrefix(req.Path, state.OrderPath)
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

func (a *app) doQuery(key base.Bytes) tmtypes.ResponseQuery {

	depo, err := a.State().Order().GetByKey(key)

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if depo == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("order %x not found", key),
		}
	}

	bytes, err := proto.Marshal(depo)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Key:    data.Bytes(a.State().Order().KeyFor(key)),
		Value:  bytes,
		Height: int64(a.State().Version()),
	}
}

func (a *app) doRangeQuery(key base.Bytes) tmtypes.ResponseQuery {
	items, err := a.State().Order().All()
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	coll := &types.Orders{Items: items}

	bytes, err := proto.Marshal(coll)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Key:    data.Bytes(state.OrderPath),
		Value:  bytes,
		Height: a.State().Version(),
	}
}

func (a *app) doCheckCreateTx(ctx apptypes.Context, tx *types.TxCreateOrder) tmtypes.ResponseCheckTx {

	// todo: ensure signed by last block creator / valid market facilitator

	// ensure order provided
	order := tx.Order
	if order == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "No order specified",
		}
	}

	// ensure deployment exists
	deployment, err := a.State().Deployment().Get(order.Deployment)
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

	// ensure deployment group exists
	group, err := a.State().DeploymentGroup().Get(order.Deployment, order.Group)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if group == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Group not found",
		}
	}

	// ensure deployment group in correct state
	if group.GetState() != types.DeploymentGroup_OPEN {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Group not in open state",
		}
	}

	// ensure no other open orders
	others, err := a.State().Order().ForGroup(group)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	for _, other := range others {
		if other.GetState() == types.Order_OPEN {
			return tmtypes.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "Order already exists for group",
			}
		}
	}

	return tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverCreateTx(ctx apptypes.Context, tx *types.TxCreateOrder) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckCreateTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	order := tx.Order

	oseq := a.State().Deployment().SequenceFor(order.Deployment)
	oseq.Advance()
	// order.Order = oseq.Advance()
	order.State = types.Order_OPEN

	if err := a.State().Order().Save(order); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateOrder),
	}
}

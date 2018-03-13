package fulfillment

import (
	"bytes"
	"strings"

	"github.com/tendermint/tmlibs/log"

	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/ovrclk/photon/types/code"
	tmtypes "github.com/tendermint/abci/types"
)

const (
	Name = apptypes.TagAppFulfillment
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(state state.State, log log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, state, log)}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), state.FulfillmentPath)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return a.doCheckTx(ctx, tx.TxCreateFulfillment)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return a.doDeliverTx(ctx, tx.TxCreateFulfillment)
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
	id := strings.TrimPrefix(req.Path, state.FulfillmentPath)
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

func (a *app) doCheckTx(ctx apptypes.Context, tx *types.TxCreateFulfillment) tmtypes.ResponseCheckTx {
	order := tx.GetOrder()

	if order == nil {
		return tmtypes.ResponseCheckTx{Code: code.INVALID_TRANSACTION}
	}

	// lookup provider
	provider, err := a.State().Provider().Get(order.Provider)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if provider == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "provider not found",
		}
	}

	// ensure tx signed by provider account
	if !bytes.Equal(ctx.Signer().Address(), provider.Owner) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by provider owner",
		}
	}

	// ensure provider account exists
	acct, err := a.State().Account().Get(provider.Owner)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if acct == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Provider account not found",
		}
	}

	// ensure order exists
	dorder, err := a.State().Order().Get(order.Deployment, order.Group, order.Order)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if dorder == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not found",
		}
	}

	// ensure order in correct state
	if dorder.State != types.Order_OPEN {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not open",
		}
	}

	// ensure there are no other orders for this provider
	other, err := a.State().Fulfillment().Get(order.Deployment, order.Group, order.Order, order.Provider)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if other != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillment order by provider already exists.",
		}
	}

	return tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverTx(ctx apptypes.Context, tx *types.TxCreateFulfillment) tmtypes.ResponseDeliverTx {
	cresp := a.doCheckTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	if err := a.State().Fulfillment().Save(tx.GetOrder()); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateFulfillment),
	}
}

func (a *app) doQuery(key base.Bytes) tmtypes.ResponseQuery {
	return tmtypes.ResponseQuery{}
}

func (a *app) doRangeQuery(key base.Bytes) tmtypes.ResponseQuery {
	return tmtypes.ResponseQuery{}
}

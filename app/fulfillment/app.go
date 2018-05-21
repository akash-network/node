package fulfillment

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tendermint/tmlibs/log"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
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
	case *types.TxPayload_TxCloseFulfillment:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return a.doCheckCreateTx(ctx, tx.TxCreateFulfillment)
	case *types.TxPayload_TxCloseFulfillment:
		_, resp := a.doCheckCloseTx(ctx, tx.TxCloseFulfillment)
		return resp
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return a.doDeliverCreateTx(ctx, tx.TxCreateFulfillment)
	case *types.TxPayload_TxCloseFulfillment:
		return a.doDeliverCloseTx(ctx, tx.TxCloseFulfillment)
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
	key, err := keys.ParseFulfillmentPath(id)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	return a.doQuery(key)
}

func (a *app) doCheckCreateTx(ctx apptypes.Context, tx *types.TxCreateFulfillment) tmtypes.ResponseCheckTx {

	if tx.Deployment == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Empty deployment",
		}
	}

	if tx.Provider == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Empty provider",
		}
	}

	// lookup provider
	provider, err := a.State().Provider().Get(tx.Provider)
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
	dorder, err := a.State().Order().Get(tx.OrderID())
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

	// get deployment group
	group, err := a.State().DeploymentGroup().Get(tx.GroupID())
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	// ensure provider has matching attributes
	for _, requirement := range group.Requirements {
		valid := false
		for _, attribute := range provider.Attributes {
			if requirement.Name == attribute.Name && requirement.Value == attribute.Value {
				valid = true
			}
		}
		if !valid {
			return tmtypes.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "Invalid provider attribute",
			}
		}
	}

	// ensure there are no other orders for this provider
	other, err := a.State().Fulfillment().Get(tx.FulfillmentID)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if other != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillment by provider already exists.",
		}
	}

	return tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverCreateTx(ctx apptypes.Context, tx *types.TxCreateFulfillment) tmtypes.ResponseDeliverTx {
	cresp := a.doCheckCreateTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	fulfillment := &types.Fulfillment{
		FulfillmentID: tx.FulfillmentID,
		State:         types.Fulfillment_OPEN,
	}

	if err := a.State().Fulfillment().Save(fulfillment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateFulfillment),
	}
}

func (a *app) doCheckCloseTx(ctx apptypes.Context, tx *types.TxCloseFulfillment) (*types.Fulfillment, tmtypes.ResponseCheckTx) {

	// lookup fulfillment
	fulfillment, err := a.State().Fulfillment().Get(tx.FulfillmentID)
	if err != nil {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if fulfillment == nil {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "fulfillment not found",
		}
	}
	if fulfillment.State != types.Fulfillment_OPEN {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "fulfillment not open",
		}
	}

	// ensure provider exists
	provider, err := a.State().Provider().Get(fulfillment.Provider)
	if err != nil {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if provider == nil {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Provider not found",
		}
	}

	// ensure ownder exists
	owner, err := a.State().Account().Get(provider.Owner)
	if err != nil {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if owner == nil {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Owner not found",
		}
	}

	// ensure tx signed by provider
	if !bytes.Equal(ctx.Signer().Address(), owner.Address) {
		return nil, tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by provider",
		}
	}

	return fulfillment, tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverCloseTx(ctx apptypes.Context, tx *types.TxCloseFulfillment) tmtypes.ResponseDeliverTx {
	fulfillment, cresp := a.doCheckCloseTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	fulfillment.State = types.Fulfillment_CLOSED

	if err := a.State().Fulfillment().Save(fulfillment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCloseFulfillment),
	}
}

func (a *app) doQuery(key keys.Fulfillment) tmtypes.ResponseQuery {
	ful, err := a.State().Fulfillment().Get(key.ID())

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if ful == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("fulfillment %v not found", key.Path()),
		}
	}

	bytes, err := proto.Marshal(ful)
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

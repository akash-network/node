package fulfillment

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tendermint/tendermint/libs/log"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	abci_types "github.com/tendermint/tendermint/abci/types"
)

const (
	Name = apptypes.TagAppFulfillment
)

type app struct {
	*apptypes.BaseApp
}

func NewApp(log log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, log)}, nil
}

func (a *app) AcceptQuery(req abci_types.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.FulfillmentPath)
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

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return a.doCheckCreateTx(state, ctx, tx.TxCreateFulfillment)
	case *types.TxPayload_TxCloseFulfillment:
		_, resp := a.doCheckCloseTx(state, ctx, tx.TxCloseFulfillment)
		return resp
	}
	return abci_types.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) abci_types.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateFulfillment:
		return a.doDeliverCreateTx(state, ctx, tx.TxCreateFulfillment)
	case *types.TxPayload_TxCloseFulfillment:
		return a.doDeliverCloseTx(state, ctx, tx.TxCloseFulfillment)
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

	// todo: abstractiion: all queries should have this
	id := strings.TrimPrefix(req.Path, appstate.FulfillmentPath)

	if len(id) == 0 {
		return a.doRangeQuery(state)
	}

	key, err := keys.ParseFulfillmentPath(id)
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	return a.doQuery(state, key)
}

func (a *app) doCheckCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateFulfillment) abci_types.ResponseCheckTx {

	if tx.Price == 0 {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillments must have a non-zero price",
		}
	}

	if tx.Deployment == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Empty deployment",
		}
	}

	if tx.Provider == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Empty provider",
		}
	}

	// lookup provider
	provider, err := state.Provider().Get(tx.Provider)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if provider == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "provider not found",
		}
	}

	// ensure tx signed by provider account
	if !bytes.Equal(ctx.Signer().Address(), provider.Owner) {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by provider owner",
		}
	}

	// ensure provider account exists
	acct, err := state.Account().Get(provider.Owner)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if acct == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Provider account not found",
		}
	}

	// ensure order exists
	dorder, err := state.Order().Get(tx.OrderID())
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if dorder == nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not found",
		}
	}

	// ensure order in correct state
	if dorder.State != types.Order_OPEN {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not open",
		}
	}

	// get deployment group
	group, err := state.DeploymentGroup().Get(tx.GroupID())
	if err != nil {
		return abci_types.ResponseCheckTx{
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
			return abci_types.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "Invalid provider attribute",
			}
		}
	}

	// ensure there are no other orders for this provider
	other, err := state.Fulfillment().Get(tx.FulfillmentID)
	if err != nil {
		return abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if other != nil {
		return abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillment by provider already exists.",
		}
	}

	return abci_types.ResponseCheckTx{}
}

func (a *app) doDeliverCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateFulfillment) abci_types.ResponseDeliverTx {
	cresp := a.doCheckCreateTx(state, ctx, tx)
	if !cresp.IsOK() {
		return abci_types.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	fulfillment := &types.Fulfillment{
		FulfillmentID: tx.FulfillmentID,
		State:         types.Fulfillment_OPEN,
		Price:         tx.Price,
	}

	if err := state.Fulfillment().Save(fulfillment); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCreateFulfillment),
	}
}

func (a *app) doCheckCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseFulfillment) (*types.Fulfillment, abci_types.ResponseCheckTx) {

	// lookup fulfillment
	fulfillment, err := state.Fulfillment().Get(tx.FulfillmentID)
	if err != nil {
		return nil, abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if fulfillment == nil {
		return nil, abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "fulfillment not found",
		}
	}
	if fulfillment.State != types.Fulfillment_OPEN {
		return nil, abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "fulfillment not open",
		}
	}

	// ensure provider exists
	provider, err := state.Provider().Get(fulfillment.Provider)
	if err != nil {
		return nil, abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if provider == nil {
		return nil, abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Provider not found",
		}
	}

	// ensure ownder exists
	owner, err := state.Account().Get(provider.Owner)
	if err != nil {
		return nil, abci_types.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if owner == nil {
		return nil, abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Owner not found",
		}
	}

	// ensure tx signed by provider
	if !bytes.Equal(ctx.Signer().Address(), owner.Address) {
		return nil, abci_types.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by provider",
		}
	}

	return fulfillment, abci_types.ResponseCheckTx{}
}

func (a *app) doDeliverCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseFulfillment) abci_types.ResponseDeliverTx {
	fulfillment, cresp := a.doCheckCloseTx(state, ctx, tx)
	if !cresp.IsOK() {
		return abci_types.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	fulfillment.State = types.Fulfillment_CLOSED

	if err := state.Fulfillment().Save(fulfillment); err != nil {
		return abci_types.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return abci_types.ResponseDeliverTx{
		Tags: apptypes.NewTags(a.Name(), apptypes.TxTypeCloseFulfillment),
	}
}

func (a *app) doRangeQuery(state appstate.State) abci_types.ResponseQuery {
	items, err := state.Fulfillment().All()
	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	coll := &types.Fulfillments{Items: items}

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

func (a *app) doQuery(state appstate.State, key keys.Fulfillment) abci_types.ResponseQuery {
	ful, err := state.Fulfillment().Get(key.ID())

	if err != nil {
		return abci_types.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if ful == nil {
		return abci_types.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("fulfillment %v not found", key.Path()),
		}
	}

	bytes, err := proto.Marshal(ful)
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

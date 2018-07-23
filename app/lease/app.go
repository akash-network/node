package lease

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/app/market"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/keys"
	appstate "github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	tmtypes "github.com/tendermint/abci/types"
	tmcommon "github.com/tendermint/tmlibs/common"
	"github.com/tendermint/tmlibs/log"
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

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), appstate.LeasePath)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateLease:
		return true
	case *types.TxPayload_TxCloseLease:
		return true
	}
	return false
}

func (a *app) CheckTx(state appstate.State, ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateLease:
		resp, _ := a.doCheckCreateTx(state, ctx, tx.TxCreateLease)
		return resp
	case *types.TxPayload_TxCloseLease:
		resp, _ := a.doCheckCloseTx(state, ctx, tx.TxCloseLease)
		return resp
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(state appstate.State, ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateLease:
		return a.doDeliverCreateTx(state, ctx, tx.TxCreateLease)
	case *types.TxPayload_TxCloseLease:
		return a.doDeliverCloseTx(state, ctx, tx.TxCloseLease)
	}
	return tmtypes.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) Query(state appstate.State, req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	if !a.AcceptQuery(req) {
		return tmtypes.ResponseQuery{
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

	return tmtypes.ResponseQuery{
		Code: code.ERROR,
		Log:  err.Error(),
	}
}

func (a *app) doCheckCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateLease) (tmtypes.ResponseCheckTx, *types.Order) {
	if tx.Deployment == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Empty deployment",
		}, nil
	}

	if tx.Provider == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Empty provider",
		}, nil
	}

	// lookup provider
	provider, err := state.Provider().Get(tx.Provider)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if provider == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "provider not found",
		}, nil
	}

	// ensure provider account exists
	acct, err := state.Account().Get(provider.Owner)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if acct == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Provider account not found",
		}, nil
	}

	// ensure order exists
	order, err := state.Order().Get(tx.OrderID())
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if order == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not found",
		}, nil
	}

	// ensure order in correct state
	if order.State != types.Order_OPEN {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not open",
		}, nil
	}

	// ensure fulfillment exists
	fulfillment, err := state.Fulfillment().Get(tx.FulfillmentID())
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if fulfillment == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillment not found",
		}, nil
	}
	if fulfillment.State != types.Fulfillment_OPEN {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillment not open",
		}, nil
	}

	bestFulfillment, err := market.BestFulfillment(state, order)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}

	if bestFulfillment.Compare(fulfillment) != 0 {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  "Unexpected fulfillment",
		}, nil
	}

	return tmtypes.ResponseCheckTx{}, order
}

func (a *app) doDeliverCreateTx(state appstate.State, ctx apptypes.Context, tx *types.TxCreateLease) tmtypes.ResponseDeliverTx {
	cresp, matchedOrder := a.doCheckCreateTx(state, ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	lease := &types.Lease{
		LeaseID: tx.LeaseID,
		Price:   tx.Price,
		State:   types.Lease_ACTIVE,
	}

	if err := state.Lease().Save(lease); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	group, err := state.DeploymentGroup().Get(tx.GroupID())
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if group == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "group not found",
		}
	}

	orders, err := state.Order().ForGroup(group.DeploymentGroupID)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if orders == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "orders not found",
		}
	}

	for _, order := range orders {
		if order.Seq != matchedOrder.Seq {
			order.State = types.Order_CLOSED
		} else {
			order.State = types.Order_MATCHED
		}
		if err := state.Order().Save(order); err != nil {
			return tmtypes.ResponseDeliverTx{
				Code: code.INVALID_TRANSACTION,
				Log:  err.Error(),
			}
		}
	}

	tags := apptypes.NewTags(a.Name(), apptypes.TxTypeCreateLease)
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameDeployment), Value: lease.Deployment})
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameLease), Value: keys.LeaseID(lease.LeaseID).Bytes()})

	return tmtypes.ResponseDeliverTx{
		Tags: tags,
	}
}

func (a *app) doCheckCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseLease) (tmtypes.ResponseCheckTx, *types.Lease) {

	// lookup provider
	lease, err := state.Lease().Get(tx.LeaseID)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if lease == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "lease not found",
		}, nil
	}

	if lease.State != types.Lease_ACTIVE {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "lease not active",
		}, nil
	}

	return tmtypes.ResponseCheckTx{}, lease
}

func (a *app) doDeliverCloseTx(state appstate.State, ctx apptypes.Context, tx *types.TxCloseLease) tmtypes.ResponseDeliverTx {
	cresp, lease := a.doCheckCloseTx(state, ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	group, err := state.DeploymentGroup().Get(lease.GroupID())
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if group == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "group not found",
		}
	}

	order, err := state.Order().Get(lease.OrderID())
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}
	if order == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "order not found",
		}
	}

	order.State = types.Order_CLOSED
	if err := state.Order().Save(order); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	group.State = types.DeploymentGroup_OPEN
	if err := state.DeploymentGroup().Save(group); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	lease.State = types.Lease_CLOSED
	if err := state.Lease().Save(lease); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	tags := apptypes.NewTags(a.Name(), apptypes.TxTypeCloseLease)
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameLease), Value: keys.LeaseID(lease.LeaseID).Bytes()})

	return tmtypes.ResponseDeliverTx{
		Tags: tags,
	}
}

func (a *app) doQuery(state appstate.State, key keys.Lease) tmtypes.ResponseQuery {
	lease, err := state.Lease().Get(key.ID())

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if lease == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("lease %v not found", key.Path()),
		}
	}

	bytes, err := proto.Marshal(lease)
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
	leases, err := state.Lease().All()
	if err != nil {
		return tmtypes.ResponseQuery{
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
		if bytes.Equal(deployment.Tenant, tenant) {
			items = append(items, lease)
		}
	}

	coll := &types.Leases{Items: items}

	bytes, err := proto.Marshal(coll)
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

func (a *app) doDeploymentQuery(state appstate.State, key keys.Deployment) tmtypes.ResponseQuery {
	items, err := state.Lease().ForDeployment(key.Bytes())
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	coll := &types.Leases{Items: items}

	bytes, err := proto.Marshal(coll)
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

// billing for leases
func ProcessLeases(state appstate.State) error {
	leases, err := state.Lease().All()
	if err != nil {
		return err
	}
	for _, lease := range leases {
		if lease.State == types.Lease_ACTIVE {
			if err := processLease(state, *lease); err != nil {
				return err
			}
		}
	}
	return nil
}

func processLease(state appstate.State, lease types.Lease) error {
	deployment, err := state.Deployment().Get(lease.Deployment)
	if err != nil {
		return err
	}
	if deployment == nil {
		return errors.New("deployment not found")
	}
	tenant, err := state.Account().Get(deployment.Tenant)
	if err != nil {
		return err
	}
	if tenant == nil {
		return errors.New("tenant not found")
	}
	provider, err := state.Provider().Get(lease.Provider)
	if err != nil {
		return err
	}
	if provider == nil {
		return errors.New("provider not found")
	}
	owner, err := state.Account().Get(provider.Owner)
	if err != nil {
		return err
	}
	if owner == nil {
		return errors.New("owner not found")
	}

	p := uint64(lease.Price)

	if tenant.Balance >= p {
		owner.Balance += p
		tenant.Balance -= p
	} else {
		owner.Balance += tenant.Balance
		tenant.Balance = 0
	}

	err = state.Account().Save(tenant)
	if err != nil {
		return err
	}

	err = state.Account().Save(owner)
	if err != nil {
		return err
	}

	return nil
}

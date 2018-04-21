package lease

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/app/market"
	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
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

func NewApp(state state.State, log log.Logger) (apptypes.Application, error) {
	return &app{apptypes.NewBaseApp(Name, state, log)}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), state.LeasePath)
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

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateLease:
		resp, _ := a.doCheckCreateTx(ctx, tx.TxCreateLease)
		return resp
	case *types.TxPayload_TxCloseLease:
		resp, _ := a.doCheckCloseTx(ctx, tx.TxCloseLease)
		return resp
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateLease:
		return a.doDeliverCreateTx(ctx, tx.TxCreateLease)
	case *types.TxPayload_TxCloseLease:
		return a.doDeliverCloseTx(ctx, tx.TxCloseLease)
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
	id := strings.TrimPrefix(req.Path, state.LeasePath)
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
	if len(id) == state.AddressSize*2 {
		return a.doDeploymentQuery(*key)
	}
	return a.doQuery(*key)
}

func (a *app) doCheckCreateTx(ctx apptypes.Context, tx *types.TxCreateLease) (tmtypes.ResponseCheckTx, *types.Order) {
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
	provider, err := a.State().Provider().Get(tx.Provider)
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
	acct, err := a.State().Account().Get(provider.Owner)
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
	order, err := a.State().Order().Get(tx.Deployment, tx.Group, tx.Order)
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
	fulfillment, err := a.State().Fulfillment().Get(tx.Deployment, tx.Group, tx.Order, tx.Provider)
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

	bestFulfillment, err := market.BestFulfillment(a.State(), order)
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

func (a *app) doDeliverCreateTx(ctx apptypes.Context, tx *types.TxCreateLease) tmtypes.ResponseDeliverTx {
	cresp, order := a.doCheckCreateTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	lease := &types.Lease{
		Deployment: tx.Deployment,
		Group:      tx.Group,
		Order:      tx.Order,
		Provider:   tx.Provider,
		Price:      tx.Price,
		State:      types.Lease_ACTIVE,
	}

	if err := a.State().Lease().Save(lease); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	order.State = types.Order_MATCHED
	if err := a.State().Order().Save(order); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	tags := apptypes.NewTags(a.Name(), apptypes.TxTypeCreateLease)
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameDeployment), Value: lease.Deployment})
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameLease), Value: state.IDForLease(lease)})

	return tmtypes.ResponseDeliverTx{
		Tags: tags,
	}
}

func (a *app) doCheckCloseTx(ctx apptypes.Context, tx *types.TxCloseLease) (tmtypes.ResponseCheckTx, *types.Lease) {
	if tx.Lease == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "empty lease",
		}, nil
	}

	// lookup provider
	lease, err := a.State().Lease().GetByKey(tx.Lease)
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

func (a *app) doDeliverCloseTx(ctx apptypes.Context, tx *types.TxCloseLease) tmtypes.ResponseDeliverTx {
	cresp, lease := a.doCheckCloseTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	lease.State = types.Lease_CLOSED

	if err := a.State().Lease().Save(lease); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	tags := apptypes.NewTags(a.Name(), apptypes.TxTypeCloseLease)
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameLease), Value: state.IDForLease(lease)})

	return tmtypes.ResponseDeliverTx{
		Tags: tags,
	}
}

func (a *app) doQuery(key base.Bytes) tmtypes.ResponseQuery {
	lease, err := a.State().Lease().GetByKey(key)

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if lease == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("lease %x not found", key),
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
		Height: a.State().Version(),
	}
}

func (a *app) doRangeQuery(key base.Bytes) tmtypes.ResponseQuery {
	items, err := a.State().Lease().All()
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
		Height: a.State().Version(),
	}
}

func (a *app) doDeploymentQuery(deployment base.Bytes) tmtypes.ResponseQuery {
	items, err := a.State().Lease().ForDeployment(deployment)
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
		Height: a.State().Version(),
	}
}

// billing for leases
func ProcessLeases(state state.State) error {
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

func processLease(state state.State, lease types.Lease) error {
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

package lease

import (
	"github.com/tendermint/tmlibs/log"

	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/code"
	tmtypes "github.com/tendermint/abci/types"
	tmcommon "github.com/tendermint/tmlibs/common"
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
	return false
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateLease:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateLease:
		resp, _ := a.doCheckTx(ctx, tx.TxCreateLease)
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
		return a.doDeliverTx(ctx, tx.TxCreateLease)
	}
	return tmtypes.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {
	return tmtypes.ResponseQuery{
		Code: code.UNKNOWN_QUERY,
		Log:  "invalid key",
	}
}

func (a *app) doCheckTx(ctx apptypes.Context, tx *types.TxCreateLease) (tmtypes.ResponseCheckTx, *types.DeploymentOrder) {
	lease := tx.GetLease()

	if lease == nil {
		return tmtypes.ResponseCheckTx{Code: code.INVALID_TRANSACTION}, nil
	}

	// lookup provider
	provider, err := a.State().Provider().Get(lease.Provider)
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

	// ensure deployment order exists
	dorder, err := a.State().DeploymentOrder().Get(lease.Deployment, lease.Group, lease.Order)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if dorder == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment order not found",
		}, nil
	}

	// ensure deployment order in correct state
	if dorder.State != types.DeploymentOrder_OPEN {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Deployment order not open",
		}, nil
	}

	// ensure fulfillment order exists
	forder, err := a.State().FulfillmentOrder().Get(lease.Deployment, lease.Group, lease.Order, lease.Provider)
	if err != nil {
		return tmtypes.ResponseCheckTx{
			Code: code.ERROR,
			Log:  err.Error(),
		}, nil
	}
	if forder == nil {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Fulfillment order not found",
		}, nil
	}

	// TODO: ensure fulfillment order in correct state
	// TODO: verify that matching algorithm would choose this match

	return tmtypes.ResponseCheckTx{}, dorder
}

func (a *app) doDeliverTx(ctx apptypes.Context, tx *types.TxCreateLease) tmtypes.ResponseDeliverTx {
	cresp, dorder := a.doCheckTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	if err := a.State().Lease().Save(tx.GetLease()); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	dorder.State = types.DeploymentOrder_MATCHED
	if err := a.State().DeploymentOrder().Save(dorder); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	lease := tx.GetLease()
	tags := apptypes.NewTags(a.Name(), apptypes.TxTypeCreateLease)
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameDeployment), Value: lease.Deployment})
	tags = append(tags, tmcommon.KVPair{Key: []byte(apptypes.TagNameLease), Value: state.IDForLease(lease)})

	return tmtypes.ResponseDeliverTx{
		Tags: tags,
	}
}

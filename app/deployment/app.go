package deployment

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/ovrclk/photon/types/code"
	tmtypes "github.com/tendermint/abci/types"
	"github.com/tendermint/go-wire/data"
	"github.com/tendermint/tmlibs/log"
)

type app struct {
	state  state.State
	logger log.Logger
}

func NewApp(state state.State, logger log.Logger) (apptypes.Application, error) {
	return &app{state, logger}, nil
}

func (a *app) AcceptQuery(req tmtypes.RequestQuery) bool {
	return strings.HasPrefix(req.GetPath(), state.DeploymentPath)
}

func (a *app) Query(req tmtypes.RequestQuery) tmtypes.ResponseQuery {

	// todo: abstractiion: all queries should have this
	if !a.AcceptQuery(req) {
		return tmtypes.ResponseQuery{
			Code: code.UNKNOWN_QUERY,
			Log:  "invalid key",
		}
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

	dep, err := a.state.Deployment().Get(*key)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if dep == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment %x not found", *key),
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
		Key:    data.Bytes(a.state.Account().KeyFor(*key)),
		Value:  bytes,
		Height: int64(a.state.Version()),
	}
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxDeployment:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxDeployment:
		return a.doCheckTxDeployment(ctx, tx.TxDeployment)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxDeployment:
		return a.doDeliverTxDeployment(ctx, tx.TxDeployment)
	}
	return tmtypes.ResponseDeliverTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

// todo: break each check out into a named global exported funtion for all trasaction types to utilize
func (a *app) doCheckTxDeployment(ctx apptypes.Context, tx *types.TxDeployment) tmtypes.ResponseCheckTx {

	// todo: abstraction - all tx handlers need this
	if !bytes.Equal(ctx.Signer().Address(), tx.From) {
		return tmtypes.ResponseCheckTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "Not signed by sending address",
		}
	}

	acct, err := a.state.Account().Get(tx.From)
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

	/* todo: balance checks
	    balance > deployment stake
	    balance > minimum deployment cost?
	    balance > ?????
		if acct.Balance < tx.Amount {
			return tmtypes.ResponseCheckTx{
				Code: code.INVALID_TRANSACTION,
				Log:  "insufficient funds",
			}
		}*/

	return tmtypes.ResponseCheckTx{}
}

// todo: rename 'doDeliverTxDeployment' to 'doDeliverTx' because simpler
func (a *app) doDeliverTxDeployment(ctx apptypes.Context, tx *types.TxDeployment) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckTxDeployment(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	acct, err := a.state.Account().Get(tx.From)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}
	if acct == nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  "unknown source account",
		}
	}

	deployment := tx.Deployment

	// todo / question/ idea: hold deployment stake in "escrow" - bind to deployment?
	// acct.Balance -= tx.Stake
	// deployment.Balance += tx.Stake
	// if deployment is canceled -> acct.Balance += deployment.Balance && rm deployment

	if err := a.state.Account().Save(acct); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	if err := a.state.Deployment().Save(deployment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{}
}

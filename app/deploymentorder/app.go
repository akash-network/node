package deploymentorder

import (
	"encoding/binary"
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
	return strings.HasPrefix(req.GetPath(), state.DeploymentOrderPath)
}

func (a *app) AcceptTx(ctx apptypes.Context, tx interface{}) bool {
	switch tx.(type) {
	case *types.TxPayload_TxCreateDeploymentOrder:
		return true
	}
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeploymentOrder:
		return a.doCheckTx(ctx, tx.TxCreateDeploymentOrder)
	}
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
	switch tx := tx.(type) {
	case *types.TxPayload_TxCreateDeploymentOrder:
		return a.doDeliverTx(ctx, tx.TxCreateDeploymentOrder)
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
	id := strings.TrimPrefix(req.Path, state.DeploymentOrderPath)
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

	depo, err := a.state.DeploymentOrder().Get(key)

	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if depo == nil {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment order %x not found", key),
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
		Key:    data.Bytes(a.state.DeploymentOrder().KeyFor(key)),
		Value:  bytes,
		Height: int64(a.state.Version()),
	}
}

func (a *app) doRangeQuery(key base.Bytes) tmtypes.ResponseQuery {
	depos, err := a.state.DeploymentOrder().GetMaxRange()
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	if len(depos.DeploymentOrders) == 0 {
		return tmtypes.ResponseQuery{
			Code: code.NOT_FOUND,
			Log:  fmt.Sprintf("deployment orders not found"),
		}
	}

	bytes, err := proto.Marshal(depos)
	if err != nil {
		return tmtypes.ResponseQuery{
			Code: code.ERROR,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseQuery{
		Key:    data.Bytes(state.DeploymentOrderPath),
		Value:  bytes,
		Height: int64(a.state.Version()),
	}
}

// todo: break each type of check out into a named global exported funtion for all trasaction types to utilize
func (a *app) doCheckTx(ctx apptypes.Context, tx *types.TxCreateDeploymentOrder) tmtypes.ResponseCheckTx {

	// todo: ensure signed by last block creator / valid market facilitator
	return tmtypes.ResponseCheckTx{}
}

func (a *app) doDeliverTx(ctx apptypes.Context, tx *types.TxCreateDeploymentOrder) tmtypes.ResponseDeliverTx {

	cresp := a.doCheckTx(ctx, tx)
	if !cresp.IsOK() {
		return tmtypes.ResponseDeliverTx{
			Code: cresp.Code,
			Log:  cresp.Log,
		}
	}

	deploymentOrder := tx.DeploymentOrder

	deployment, err := a.state.Deployment().Get(deploymentOrder.Deployment)
	if err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	if err := a.state.DeploymentOrder().Save(deploymentOrder); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	deployment.Groups[deploymentOrder.GroupIndex].State = types.DeploymentGroup_ORDERED
	if err := a.state.Deployment().Save(deployment); err != nil {
		return tmtypes.ResponseDeliverTx{
			Code: code.INVALID_TRANSACTION,
			Log:  err.Error(),
		}
	}

	return tmtypes.ResponseDeliverTx{}
}

func CreateDeploymentOrderTxs(state state.State) ([]types.TxCreateDeploymentOrder, error) {
	depotxs := make([]types.TxCreateDeploymentOrder, 0, 0)
	deps, err := state.Deployment().GetMaxRange()
	if err != nil {
		return depotxs, err
	}

	for _, deployment := range deps.Deployments {
		if deployment.State == types.Deployment_ACTIVE {
			for i, group := range deployment.Groups {
				if group.State == types.DeploymentGroup_OPEN {
					ibytes := make([]byte, binary.MaxVarintLen32)
					binary.PutUvarint(ibytes, uint64(i))
					abytes := make([]byte, 32)
					_, err := deployment.Address.MarshalTo(abytes)
					if err != nil {
						return depotxs, err
					}
					depotx := &types.TxCreateDeploymentOrder{
						DeploymentOrder: &types.DeploymentOrder{
							Address:    append(abytes, ibytes...),
							Deployment: abytes,
							GroupIndex: uint32(i),
							State:      types.DeploymentOrder_OPEN,
						},
					}
					depotxs = append(depotxs, *depotx)
				}
			}
		}
	}
	return depotxs, nil
}

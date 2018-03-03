package deploymentOrder

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
	return false
}

func (a *app) CheckTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseCheckTx {
	return tmtypes.ResponseCheckTx{
		Code: code.UNKNOWN_TRANSACTION,
		Log:  "unknown transaction",
	}
}

func (a *app) DeliverTx(ctx apptypes.Context, tx interface{}) tmtypes.ResponseDeliverTx {
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

func CreateDeploymentOrderTxs(state state.State) ([]types.TxCreateDeploymentOrder, error) {
	depotxs := make([]types.TxCreateDeploymentOrder, 0, 0)
	deps, err := state.Deployment().GetMaxRange()
	if err != nil {
		return depotxs, err
	}

	for _, deployment := range deps.Deployments {
		if deployment.State == types.Deployment_ACTIVE {
			println("found active deployment", deployment.Address.EncodeString())
			for i, group := range deployment.Groups {
				if group.State == types.DeploymentGroup_OPEN {

					// create deploymentOrder for group
					ibytes := make([]byte, binary.MaxVarintLen32)
					binary.PutUvarint(ibytes, uint64(i))

					depotx := &types.TxCreateDeploymentOrder{
						DeploymentOrder: &types.DeploymentOrder{
							Address:    append(deployment.Address, ibytes...),
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

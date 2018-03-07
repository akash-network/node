package deploymentorder_test

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ovrclk/photon/app/deployment"
	"github.com/ovrclk/photon/app/deploymentorder"
	apptypes "github.com/ovrclk/photon/app/types"
	pstate "github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtypes "github.com/tendermint/abci/types"
)

func makeDeployment(address []byte, from base.Bytes) *types.Deployment {

	const (
		name     = "region"
		value    = "us-west"
		number   = uint32(1)
		number64 = uint64(1)
	)

	resourceunit := &types.ResourceUnit{
		Cpu:    number,
		Memory: number,
		Disk:   number64,
	}

	resourcegroup := &types.ResourceGroup{
		Unit:  *resourceunit,
		Count: number,
		Price: number,
	}

	providerattribute := &types.ProviderAttribute{
		Name:  name,
		Value: value,
	}

	requirements := []types.ProviderAttribute{*providerattribute}
	resources := []types.ResourceGroup{*resourcegroup}

	activedeploymentgroup := &types.DeploymentGroup{
		Requirements: requirements,
		Resources:    resources,
		State:        types.DeploymentGroup_OPEN,
	}

	ordereddeploymentgroup := &types.DeploymentGroup{
		Requirements: requirements,
		Resources:    resources,
		State:        types.DeploymentGroup_ORDERED,
	}

	groups := []types.DeploymentGroup{
		*activedeploymentgroup,
		*ordereddeploymentgroup,
		*activedeploymentgroup,
	}

	return &types.Deployment{
		Address: address,
		From:    from,
		Groups:  groups,
	}
}

func TestDeploymentOrderApp(t *testing.T) {

	kmgr := testutil.KeyManager(t)

	key, _, err := kmgr.Create("key", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(key.Address), Balance: 0},
		},
	})

	app, err := deploymentorder.NewApp(state, testutil.Logger())
	require.NoError(t, err, "failed to create app")

	{
		data := make([]byte, 0)
		path := "/deploymentorders/"
		prove := false
		height := int64(0)
		query := tmtypes.RequestQuery{
			Data:   data,
			Path:   path,
			Height: height,
			Prove:  prove,
		}
		res := app.AcceptQuery(query)
		assert.True(t, res, "app rejcted valid query")
	}

	{
		data := make([]byte, 0)
		path := "/deployments/"
		prove := false
		height := int64(0)
		query := tmtypes.RequestQuery{
			Data:   data,
			Path:   path,
			Height: height,
			Prove:  prove,
		}
		res := app.AcceptQuery(query)
		assert.False(t, res, "app accepted invalid query")
	}

	{
		pubkey := base.PubKey(key.PubKey)
		address := base.Bytes("address")
		deploymentAddress := base.Bytes("deploymentaddress")

		{
			deploymenttx := &types.TxPayload_TxCreateDeployment{
				TxCreateDeployment: &types.TxCreateDeployment{
					Deployment: makeDeployment(deploymentAddress, base.Bytes(key.Address.Bytes())),
				},
			}

			ctx := apptypes.NewContext(&types.Tx{
				Key: &pubkey,
				Payload: types.TxPayload{
					Payload: deploymenttx,
				},
			})
			depapp, err := deployment.NewApp(state, testutil.Logger())
			require.NoError(t, err, "failed to create app")
			resp := depapp.DeliverTx(ctx, deploymenttx)
			assert.True(t, resp.IsOK())
		}

		tx := &types.TxPayload_TxCreateDeploymentOrder{
			TxCreateDeploymentOrder: &types.TxCreateDeploymentOrder{
				DeploymentOrder: &types.DeploymentOrder{
					Address:    address,
					Deployment: deploymentAddress,
					GroupIndex: 0,
					State:      types.DeploymentOrder_OPEN,
				},
			},
		}

		ctx := apptypes.NewContext(&types.Tx{
			Key: &pubkey,
			Payload: types.TxPayload{
				Payload: tx,
			},
		})

		badtx := &types.TxPayload_TxCreateDeploymentOrder{
			TxCreateDeploymentOrder: &types.TxCreateDeploymentOrder{
				DeploymentOrder: &types.DeploymentOrder{
					Address:    address,
					Deployment: address,
					GroupIndex: 0,
					State:      types.DeploymentOrder_OPEN,
				},
			},
		}

		badctx := apptypes.NewContext(&types.Tx{
			Key: &pubkey,
			Payload: types.TxPayload{
				Payload: badtx,
			},
		})

		resacc := app.AcceptTx(ctx, tx)
		assert.True(t, resacc)

		rescheck := app.CheckTx(ctx, tx)
		assert.True(t, rescheck.IsOK())
		assert.False(t, rescheck.IsErr())

		respdel := app.DeliverTx(ctx, tx)
		assert.True(t, respdel.IsOK())
		assert.False(t, respdel.IsErr())

		respdelbad := app.DeliverTx(badctx, badtx)
		assert.False(t, respdelbad.IsOK())
		assert.True(t, respdelbad.IsErr())

		{
			resp := app.Query(tmtypes.RequestQuery{Path: fmt.Sprintf("%v%v", pstate.DeploymentOrderPath, hex.EncodeToString(address))})
			assert.Empty(t, resp.Log)
			require.True(t, resp.IsOK())

			depo := new(types.DeploymentOrder)
			require.NoError(t, depo.Unmarshal(resp.Value))

			assert.Equal(t, hex.EncodeToString(deploymentAddress), hex.EncodeToString(depo.Deployment), "deployment address wrong")
			assert.Equal(t, hex.EncodeToString(address), hex.EncodeToString(depo.Address), "address wrong")
			assert.Equal(t, uint32(0x0), depo.GroupIndex, "GroupIndex wrong")
			assert.Equal(t, types.DeploymentOrder_OPEN, depo.State, "state wrong")
		}
	}

	{
		pubkey := base.PubKey(key.PubKey)
		address := base.Bytes("address")

		tx := &types.TxPayload_TxCreateDeployment{
			TxCreateDeployment: &types.TxCreateDeployment{
				Deployment: makeDeployment(address, base.Bytes(key.Address.Bytes())),
			},
		}

		ctx := apptypes.NewContext(&types.Tx{
			Key: &pubkey,
			Payload: types.TxPayload{
				Payload: tx,
			},
		})
		resacc := app.AcceptTx(ctx, tx)
		assert.False(t, resacc)

		rescheck := app.CheckTx(ctx, tx)
		assert.False(t, rescheck.IsOK())
		assert.True(t, rescheck.IsErr())

		resp := app.DeliverTx(ctx, tx)
		assert.False(t, resp.IsOK())
		assert.True(t, resp.IsErr())
	}

	{

		state = testutil.NewState(t, &types.Genesis{
			Accounts: []types.Account{
				types.Account{Address: base.Bytes(key.Address), Balance: 0},
			},
		})

		pubkey := base.PubKey(key.PubKey)
		deploymentAddress := base.Bytes("deploymentaddress")

		notxs, err := deploymentorder.CreateDeploymentOrderTxs(state)
		assert.Len(t, notxs, 0, "length not 0")
		assert.Nil(t, err, "was error")

		{
			deploymenttx := &types.TxPayload_TxCreateDeployment{
				TxCreateDeployment: &types.TxCreateDeployment{
					Deployment: makeDeployment(deploymentAddress, base.Bytes(key.Address.Bytes())),
				},
			}

			ctx := apptypes.NewContext(&types.Tx{
				Key: &pubkey,
				Payload: types.TxPayload{
					Payload: deploymenttx,
				},
			})
			depapp, err := deployment.NewApp(state, testutil.Logger())
			require.NoError(t, err, "failed to create app")
			resp := depapp.DeliverTx(ctx, deploymenttx)
			assert.True(t, resp.IsOK())
		}

		depaddr := hex.EncodeToString(deploymentAddress)
		addrzero := make([]byte, 32)
		addrtwo := make([]byte, 32)

		zerob := make([]byte, binary.MaxVarintLen32)
		twob := make([]byte, binary.MaxVarintLen32)
		binary.PutUvarint(zerob, uint64(0))
		binary.PutUvarint(twob, uint64(2))

		_, err = deploymentAddress.MarshalTo(addrzero)
		require.NoError(t, err, "failed to marshal address")
		_, err = deploymentAddress.MarshalTo(addrtwo)
		require.NoError(t, err, "failed to marshal address")

		addrzero = append(addrzero, zerob...)
		addrtwo = append(addrtwo, twob...)

		txs, err := deploymentorder.CreateDeploymentOrderTxs(state)
		assert.Nil(t, err, "was error")
		assert.Len(t, txs, 2, "length not 2")

		assert.Equal(t, depaddr, hex.EncodeToString(txs[0].DeploymentOrder.Deployment), "deployment address wrong")
		assert.Equal(t, hex.EncodeToString(addrzero), hex.EncodeToString(txs[0].DeploymentOrder.Address), "address wrong")
		assert.Equal(t, uint32(0x0), txs[0].DeploymentOrder.GroupIndex, "GroupIndex wrong")
		assert.Equal(t, types.DeploymentOrder_OPEN, txs[0].DeploymentOrder.State, "state wrong")

		assert.Equal(t, depaddr, hex.EncodeToString(txs[1].DeploymentOrder.Deployment), "deployment address wrong")
		assert.Equal(t, hex.EncodeToString(addrtwo), hex.EncodeToString(txs[1].DeploymentOrder.Address), "address wrong")
		assert.Equal(t, uint32(0x2), txs[1].DeploymentOrder.GroupIndex, "GroupIndex wrong")
		assert.Equal(t, types.DeploymentOrder_OPEN, txs[1].DeploymentOrder.State, "state wrong")
	}
}

/*

	things to test

		CreateDeploymentOrderTxs

				no deployment transactions at all to find
				no ACTIVE deployments to create stuff for
				deployment with no deployment groups
				deployment with no OPEN deployment groups
*/

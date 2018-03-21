package testutil

import (
	"fmt"
	"math/rand"
	"testing"

	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/go-crypto"
)

func RandUint32() uint32 {
	return uint32(rand.Int31())
}

func RandUint64() uint64 {
	return uint64(rand.Int63())
}

func CreateDeployment(t *testing.T, app apptypes.Application, account *types.Account, key *crypto.PrivKey, nonce uint64) *types.Deployment {
	deployment := Deployment(account.Address, nonce)

	deploymenttx := &types.TxPayload_TxCreateDeployment{
		TxCreateDeployment: &types.TxCreateDeployment{
			Deployment: deployment,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: deploymenttx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, deploymenttx))
	cresp := app.CheckTx(ctx, deploymenttx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, deploymenttx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
	return deployment
}

func CloseDeployment(t *testing.T, app apptypes.Application, deployment *base.Bytes, key *crypto.PrivKey) {

	tx := &types.TxPayload_TxCloseDeployment{
		TxCloseDeployment: &types.TxCloseDeployment{
			Deployment: *deployment,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: tx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, tx))
	cresp := app.CheckTx(ctx, tx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, tx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
}

func DeploymentClosed(t *testing.T, app apptypes.Application, deployment *base.Bytes, key *crypto.PrivKey) {
	tx := &types.TxPayload_TxDeploymentClosed{
		TxDeploymentClosed: &types.TxDeploymentClosed{
			Deployment: *deployment,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: tx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, tx))
	cresp := app.CheckTx(ctx, tx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, tx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
}

func Deployment(tenant base.Bytes, nonce uint64) *types.Deployment {

	address := state.DeploymentAddress(tenant, nonce)
	nonce++

	runit := types.ResourceUnit{
		Cpu:    RandUint32(),
		Memory: RandUint32(),
		Disk:   RandUint64(),
	}

	rgroup := types.ResourceGroup{
		Unit:  runit,
		Count: RandUint32(),
		Price: RandUint32(),
	}

	pattr := types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}

	group := types.DeploymentGroup{
		Deployment:   address,
		Seq:          nonce,
		Resources:    []types.ResourceGroup{rgroup},
		Requirements: []types.ProviderAttribute{pattr},
	}

	groups := []types.DeploymentGroup{group}

	return &types.Deployment{
		Tenant:  tenant,
		Address: address,
		Groups:  groups,
	}
}

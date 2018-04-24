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
	return uint32(rand.Int31n(100))
}

func RandUint64() uint64 {
	return uint64(rand.Int63n(100))
}

func CreateDeployment(t *testing.T, app apptypes.Application, account *types.Account, key crypto.PrivKey, nonce uint64) (*types.Deployment, *types.DeploymentGroups) {
	deployment := Deployment(account.Address, nonce)
	groups := DeploymentGroups(deployment.Address, nonce)
	ttl := int64(5)
	specs := []*types.GroupSpec{}

	for _, group := range groups.GetItems() {
		s := &types.GroupSpec{
			Resources:    group.Resources,
			Requirements: group.Requirements,
		}
		specs = append(specs, s)
	}

	deploymenttx := &types.TxPayload_TxCreateDeployment{
		TxCreateDeployment: &types.TxCreateDeployment{
			Tenant:   account.Address,
			Nonce:    nonce,
			OrderTTL: ttl,
			Groups:   specs,
		},
	}

	ctx := apptypes.NewContext(&types.Tx{
		Key: key.PubKey().Bytes(),
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
	return deployment, groups
}

func CloseDeployment(t *testing.T, app apptypes.Application, deployment *base.Bytes, key crypto.PrivKey) {

	tx := &types.TxPayload_TxCloseDeployment{
		TxCloseDeployment: &types.TxCloseDeployment{
			Deployment: *deployment,
			Reason:     types.TxCloseDeployment_TENANT_CLOSE,
		},
	}

	ctx := apptypes.NewContext(&types.Tx{
		Key: key.PubKey().Bytes(),
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
	return &types.Deployment{
		Tenant:  tenant,
		Address: state.DeploymentAddress(tenant, nonce),
	}
}

func DeploymentGroups(deployment base.Bytes, nonce uint64) *types.DeploymentGroups {
	orderTTL := int64(5)
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

	group := &types.DeploymentGroup{
		Deployment:   deployment,
		Seq:          nonce,
		Resources:    []types.ResourceGroup{rgroup},
		Requirements: []types.ProviderAttribute{pattr},
		OrderTTL:     orderTTL,
	}

	groups := []*types.DeploymentGroup{group}

	return &types.DeploymentGroups{Items: groups}
}

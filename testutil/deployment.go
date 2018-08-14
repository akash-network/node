package testutil

import (
	"fmt"
	"strconv"
	"testing"

	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/ovrclk/akash/types/unit"
	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/go-crypto"
)

func CreateDeployment(t *testing.T, st state.State, app apptypes.Application, account *types.Account, key crypto.PrivKey, nonce uint64) (*types.Deployment, *types.DeploymentGroups) {
	deployment := Deployment(account.Address, nonce)
	groups := DeploymentGroups(deployment.Address, nonce)
	ttl := int64(5)
	specs := []*types.GroupSpec{}

	for _, group := range groups.GetItems() {
		s := &types.GroupSpec{
			Name:         group.Name,
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
			Version:  Address(t),
		},
	}

	ctx := apptypes.NewContext(&types.Tx{
		Key: key.PubKey().Bytes(),
		Payload: types.TxPayload{
			Payload: deploymenttx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, deploymenttx))
	cresp := app.CheckTx(st, ctx, deploymenttx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(st, ctx, deploymenttx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())

	deployment, err := st.Deployment().Get(deployment.Address)
	assert.NoError(t, err)

	dgroups, err := st.DeploymentGroup().ForDeployment(deployment.Address)
	assert.NoError(t, err)

	return deployment, &types.DeploymentGroups{Items: dgroups}
}

func UpdateDeployment(t *testing.T,
	st state.State,
	app apptypes.Application,
	key crypto.PrivKey,
	nonce uint64,
	daddr []byte) *types.TxUpdateDeployment {

	itx := &types.TxUpdateDeployment{
		Deployment: daddr,
		Version:    Address(t),
	}

	otx := &types.TxPayload_TxUpdateDeployment{
		TxUpdateDeployment: itx,
	}

	ctx := apptypes.NewContext(&types.Tx{
		Key: key.PubKey().Bytes(),
		Payload: types.TxPayload{
			Payload: otx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, otx))
	cresp := app.CheckTx(st, ctx, otx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(st, ctx, otx)
	assert.Len(t, dresp.Log, 0, "Log should be empty but is: %v", dresp.Log)
	assert.True(t, dresp.IsOK())

	return itx
}

func CloseDeployment(t *testing.T, st state.State, app apptypes.Application, deployment *base.Bytes, key crypto.PrivKey) {

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
	cresp := app.CheckTx(st, ctx, tx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(st, ctx, tx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
}

func Deployment(tenant base.Bytes, nonce uint64, version ...[]byte) *types.Deployment {
	hash := []byte{}
	if len(version) > 0 {
		hash = version[0]
	}
	return &types.Deployment{
		Tenant:  tenant,
		Address: state.DeploymentAddress(tenant, nonce),
		Version: hash,
	}
}

func ResourceUnit() types.ResourceUnit {
	return types.ResourceUnit{
		CPU:    500,
		Memory: 256 * unit.Mi,
		Disk:   1 * unit.Gi,
	}
}

func ResourceGroup() types.ResourceGroup {
	return types.ResourceGroup{
		Unit:  ResourceUnit(),
		Count: 2,
		Price: 35,
	}
}

func DeploymentGroup(daddr []byte, nonce uint64) *types.DeploymentGroup {
	const orderTTL = int64(5)

	rgroup := ResourceGroup()
	pattr := types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}

	return &types.DeploymentGroup{
		Name: strconv.FormatUint(nonce, 10),
		DeploymentGroupID: types.DeploymentGroupID{
			Deployment: daddr,
			Seq:        nonce,
		},
		Resources:    []types.ResourceGroup{rgroup},
		Requirements: []types.ProviderAttribute{pattr},
		OrderTTL:     orderTTL,
	}
}

func DeploymentGroups(deployment base.Bytes, nonce uint64) *types.DeploymentGroups {
	return &types.DeploymentGroups{
		Items: []*types.DeploymentGroup{DeploymentGroup(deployment, nonce)},
	}
}

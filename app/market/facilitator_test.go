package market_test

import (
	"os"
	"testing"

	"github.com/ovrclk/photon/app/deployment"
	"github.com/ovrclk/photon/app/market"
	apptypes "github.com/ovrclk/photon/app/types"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtmtypes "github.com/tendermint/tendermint/types"
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

func TestFacilitatorApp(t *testing.T) {
	basedir := testutil.TempDir(t)
	defer os.RemoveAll(basedir)

	kmgr := testutil.KeyManager(t)

	key, _, err := kmgr.Create("key", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(key.Address), Balance: 0},
		},
	})

	cfg := testutil.TMConfig(t, basedir)
	logger := testutil.Logger()

	eventBus := tmtmtypes.NewEventBus()
	eventBus.SetLogger(logger.With("module", "events"))

	validator := tmtmtypes.LoadOrGenPrivValidatorFS(cfg.PrivValidatorFile())

	facilitator, err := market.NewFacilitator(logger, validator, eventBus)
	require.NoError(t, err)

	{
		pubkey := base.PubKey(key.PubKey)
		deploymentAddress := base.Bytes("deploymentaddress")

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

		err = facilitator.OnCommit(state)
		assert.NoError(t, err, "on commit errored")
	}
}

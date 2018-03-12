package testutil

import (
	"math/rand"
	"testing"

	"github.com/ovrclk/photon/state"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
)

func RandUint32() uint32 {
	return uint32(rand.Int31())
}

func RandUint64() uint64 {
	return uint64(rand.Int63())
}

func Deployment(t *testing.T, tenant base.Bytes, nonce uint64) *types.Deployment {

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

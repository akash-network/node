package testutil

import (
	"testing"

	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/go-crypto"
)

func CreateFulfillment(t *testing.T, app apptypes.Application, account *types.Account, key *crypto.PrivKey, tenant *types.Account, nonce uint64) *types.Fulfillment {
	fulfillment := Fulfillment(account.Address, tenant.Address, nonce)

	fulfillmenttx := &types.TxPayload_TxCreateFulfillment{
		TxCreateFulfillment: &types.TxCreateFulfillment{
			Fulfillment: fulfillment,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: fulfillmenttx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, fulfillmenttx))
	cresp := app.CheckTx(ctx, fulfillmenttx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, fulfillmenttx)
	assert.True(t, dresp.IsOK())
	return fulfillment
}

func Fulfillment(provider base.Bytes, tenant base.Bytes, nonce uint64) *types.Fulfillment {

	const (
		group = uint64(1)
		order = uint64(1)
		price = uint32(1)
	)
	address := state.DeploymentAddress(tenant, nonce)

	fulfillment := &types.Fulfillment{
		Deployment: address,
		Group:      group,
		Order:      order,
		Provider:   provider,
		Price:      price,
	}

	return fulfillment
}

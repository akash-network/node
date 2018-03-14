package testutil

import (
	"fmt"
	"testing"

	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/go-crypto"
)

func CreateFulfillment(t *testing.T, app apptypes.Application, provider base.Bytes, key *crypto.PrivKey, deployment base.Bytes, group, order uint64, price uint32) *types.Fulfillment {
	fulfillment := Fulfillment(provider, deployment, group, order, price)

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
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
	return fulfillment
}

func Fulfillment(provider base.Bytes, deplyment base.Bytes, group, order uint64, price uint32) *types.Fulfillment {
	fulfillment := &types.Fulfillment{
		Deployment: deplyment,
		Group:      group,
		Order:      order,
		Provider:   provider,
		Price:      price,
	}
	return fulfillment
}

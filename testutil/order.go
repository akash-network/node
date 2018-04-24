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

func CreateOrder(t *testing.T, app apptypes.Application, account *types.Account, key crypto.PrivKey, deploymentAddress base.Bytes, groupSeq, orderSeq uint64) *types.Order {
	order := Order(deploymentAddress, groupSeq, orderSeq)

	tx := &types.TxPayload_TxCreateOrder{
		TxCreateOrder: &types.TxCreateOrder{
			Deployment: order.Deployment,
			Group:      order.Group,
			Seq:        order.Seq,
			EndAt:      order.EndAt,
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
	return order
}

func Order(deploymentAddress base.Bytes, groupSeq, orderSeq uint64) *types.Order {
	order := &types.Order{
		Deployment: deploymentAddress,
		Group:      groupSeq,
		Seq:        orderSeq,
		EndAt:      int64(0),
	}
	return order
}

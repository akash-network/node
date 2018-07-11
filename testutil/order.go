package testutil

import (
	"fmt"
	"testing"

	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/tendermint/crypto"
)

func CreateOrder(t *testing.T, st state.State, app apptypes.Application, account *types.Account, key crypto.PrivKey, deploymentAddress base.Bytes, groupSeq, orderSeq uint64) *types.Order {
	order := Order(deploymentAddress, groupSeq, orderSeq)

	tx := &types.TxPayload_TxCreateOrder{
		TxCreateOrder: &types.TxCreateOrder{
			OrderID: order.OrderID,
			EndAt:   order.EndAt,
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
	assert.Empty(t, cresp.Log)
	dresp := app.DeliverTx(st, ctx, tx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
	return order
}

func Order(deploymentAddress base.Bytes, groupSeq, orderSeq uint64) *types.Order {
	order := &types.Order{
		OrderID: types.OrderID{
			Deployment: deploymentAddress,
			Group:      groupSeq,
			Seq:        orderSeq,
		},
		EndAt: int64(0),
	}
	return order
}

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

func CreateLease(t *testing.T, app apptypes.Application, provider base.Bytes, key crypto.PrivKey, deployment base.Bytes, group, order uint64, price uint32) *types.Lease {
	lease := Lease(provider, deployment, group, order, price)

	tx := &types.TxPayload_TxCreateLease{
		TxCreateLease: &types.TxCreateLease{
			Deployment: lease.Deployment,
			Group:      lease.Group,
			Order:      lease.Order,
			Provider:   lease.Provider,
			Price:      lease.Price,
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
	return lease
}

func CloseLease(t *testing.T, app apptypes.Application, lease base.Bytes, key crypto.PrivKey) {
	tx := &types.TxPayload_TxCloseLease{
		TxCloseLease: &types.TxCloseLease{
			Lease: lease,
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

func Lease(provider base.Bytes, deplyment base.Bytes, group, order uint64, price uint32) *types.Lease {
	lease := &types.Lease{
		Deployment: deplyment,
		Group:      group,
		Order:      order,
		Provider:   provider,
		Price:      price,
	}
	return lease
}

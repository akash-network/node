package testutil

import (
	"fmt"
	"testing"

	apptypes "github.com/ovrclk/akash/app/types"
	"github.com/ovrclk/akash/state"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/assert"
	crypto "github.com/tendermint/go-crypto"
)

func CreateLease(t *testing.T, st state.State, app apptypes.Application, provider base.Bytes, key crypto.PrivKey, deployment base.Bytes, group, order, price uint64) *types.Lease {
	lease := Lease(provider, deployment, group, order, price)

	tx := &types.TxPayload_TxCreateLease{
		TxCreateLease: &types.TxCreateLease{
			LeaseID: lease.LeaseID,
			Price:   lease.Price,
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
	return lease
}

func CloseLease(t *testing.T, st state.State, app apptypes.Application, id types.LeaseID, key crypto.PrivKey) {
	tx := &types.TxPayload_TxCloseLease{
		TxCloseLease: &types.TxCloseLease{
			LeaseID: id,
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

func Lease(provider base.Bytes, deplyment base.Bytes, group, order uint64, price uint64) *types.Lease {
	lease := &types.Lease{
		LeaseID: types.LeaseID{
			Deployment: deplyment,
			Group:      group,
			Order:      order,
			Provider:   provider,
		},
		Price: price,
	}
	return lease
}

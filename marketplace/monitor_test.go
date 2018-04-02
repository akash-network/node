package marketplace_test

import (
	"context"
	"testing"

	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/marketplace/mocks"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tmtmtypes "github.com/tendermint/tendermint/types"
)

func TestMonitorMarketplace(t *testing.T) {
	bus := tmtmtypes.NewEventBus()
	require.NoError(t, bus.Start())
	defer func() { require.NoError(t, bus.Stop()) }()

	signer, _ := testutil.PrivateKeySigner(t)

	tests := []struct {
		name    string
		payload interface{}
	}{
		{"OnTxSend", &types.TxSend{}},
		{"OnTxCreateProvider", &types.TxCreateProvider{}},
		{"OnTxCreateDeployment", &types.TxCreateDeployment{}},
		{"OnTxCreateOrder", &types.TxCreateOrder{}},
		{"OnTxCreateFulfillment", &types.TxCreateFulfillment{}},
		{"OnTxCreateLease", &types.TxCreateLease{}},
		{"OnTxCloseDeployment", &types.TxCloseDeployment{}},
		{"OnTxCloseLease", &types.TxCloseLease{}},
	}

	ctx := context.Background()

	for _, test := range tests {

		h := new(mocks.Handler)
		h.On(test.name, test.payload).Return(nil).Once()

		m, err := marketplace.NewMonitor(ctx, testutil.Logger(), bus, t.Name(), h, marketplace.TxQuery())
		if !assert.NoError(t, err, test.name) {
			continue
		}

		tx, err := txutil.BuildTx(signer, 1, test.payload)
		if !assert.NoError(t, err, test.name) {
			continue
		}

		bus.PublishEventTx(tmtmtypes.EventDataTx{
			TxResult: tmtmtypes.TxResult{
				Tx: tx,
			},
		})

		testutil.SleepForThreadStart(t)

		if !assert.NoError(t, m.Stop()) {
			continue
		}

		h.AssertExpectations(t)
	}

}

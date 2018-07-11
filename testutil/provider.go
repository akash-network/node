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

func CreateProvider(t *testing.T, st state.State, app apptypes.Application, account *types.Account, key crypto.PrivKey, nonce uint64) *types.Provider {
	tx := ProviderTx(account, key, nonce)
	ctx := apptypes.NewContext(tx)
	assert.True(t, app.AcceptTx(ctx, tx.Payload.Payload))
	cresp := app.CheckTx(st, ctx, tx.Payload.Payload)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(st, ctx, tx.Payload.Payload)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
	return &types.Provider{
		Address:    state.ProviderAddress(account.Address, nonce),
		Attributes: tx.Payload.GetTxCreateProvider().Attributes,
		Owner:      account.Address,
	}
}

func ProviderTx(account *types.Account, key crypto.PrivKey, nonce uint64) *types.Tx {
	provider := Provider(account.Address, nonce)
	return &types.Tx{
		Key: key.PubKey().Bytes(),
		Payload: types.TxPayload{
			Payload: &types.TxPayload_TxCreateProvider{
				TxCreateProvider: &types.TxCreateProvider{
					Attributes: provider.Attributes,
					HostURI:    "http//localhost:3000",
					Owner:      provider.Owner,
					Nonce:      nonce,
				},
			},
		},
	}
}

func Provider(account base.Bytes, nonce uint64) *types.Provider {

	address := state.ProviderAddress(account, nonce)

	providerattribute := &types.ProviderAttribute{
		Name:  "region",
		Value: "us-west",
	}

	attributes := []types.ProviderAttribute{*providerattribute}

	provider := &types.Provider{
		Address:    address,
		Attributes: attributes,
		Owner:      account,
	}

	return provider
}

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

func CreateProvider(t *testing.T, app apptypes.Application, account *types.Account, key *crypto.PrivKey, nonce uint64) *types.Provider {
	provider := Provider(account.Address, nonce)

	providertx := &types.TxPayload_TxCreateProvider{
		TxCreateProvider: &types.TxCreateProvider{
			Provider: *provider,
		},
	}

	pubkey := base.PubKey(key.PubKey())

	ctx := apptypes.NewContext(&types.Tx{
		Key: &pubkey,
		Payload: types.TxPayload{
			Payload: providertx,
		},
	})

	assert.True(t, app.AcceptTx(ctx, providertx))
	cresp := app.CheckTx(ctx, providertx)
	assert.True(t, cresp.IsOK())
	dresp := app.DeliverTx(ctx, providertx)
	assert.Len(t, dresp.Log, 0, fmt.Sprint("Log should be empty but is: ", dresp.Log))
	assert.True(t, dresp.IsOK())
	return provider
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

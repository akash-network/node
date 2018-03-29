package app_test

import (
	"testing"

	app_ "github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/code"
	"github.com/stretchr/testify/require"
)

func TestApp(t *testing.T) {

	const balance = 1

	nonce := uint64(1)

	signer, keyfrom := testutil.PrivateKeySigner(t)
	addrfrom := keyfrom.PubKey().Address().Bytes()

	keyto := testutil.PrivateKey(t)
	addrto := keyto.PubKey().Address().Bytes()

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: addrfrom, Balance: balance, Nonce: nonce},
		},
	})

	app, err := app_.Create(state, testutil.Logger())
	require.NoError(t, err)

	{
		nonce := uint64(0)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.DeliverTx(tx)
		require.Equal(t, code.INVALID_TRANSACTION, resp.Code)
		require.True(t, resp.IsErr())
		require.False(t, resp.IsOK())
	}

	{
		nonce := uint64(1)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.DeliverTx(tx)
		require.Equal(t, code.INVALID_TRANSACTION, resp.Code)
		require.True(t, resp.IsErr())
		require.False(t, resp.IsOK())
	}

	{
		nonce := uint64(2)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.DeliverTx(tx)
		require.Equal(t, code.OK, resp.Code)
		require.False(t, resp.IsErr())
		require.True(t, resp.IsOK())
	}

	{
		nonce := uint64(2)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   addrfrom,
			To:     addrto,
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.CheckTx(tx)
		require.Equal(t, code.OK, resp.Code)
		require.False(t, resp.IsErr())
		require.True(t, resp.IsOK())
	}

}

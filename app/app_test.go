package app_test

import (
	"testing"

	app_ "github.com/ovrclk/photon/app"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/ovrclk/photon/types/code"
	"github.com/stretchr/testify/require"
)

func TestApp(t *testing.T) {

	const balance = 1

	nonce := uint64(1)

	signer, keyfrom := testutil.PrivateKeySigner(t)
	keyf := base.PubKey(keyfrom.PubKey())

	keyto := testutil.PrivateKey(t)
	keyt := base.PubKey(keyto.PubKey())

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(keyf.Address()), Balance: balance, Nonce: nonce},
		},
	})

	app, err := app_.Create(state, testutil.Logger())
	require.NoError(t, err)

	{
		nonce := uint64(0)
		tx, err := txutil.BuildTx(signer, nonce, &types.TxSend{
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
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
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
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
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
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
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.CheckTx(tx)
		require.Equal(t, code.OK, resp.Code)
		require.False(t, resp.IsErr())
		require.True(t, resp.IsOK())
	}

}

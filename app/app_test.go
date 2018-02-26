package app_test

import (
	app_ "github.com/ovrclk/photon/app"
	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestApp(t *testing.T) {

	const balance = 1

	kmgr := testutil.KeyManager(t)
	nonce := uint64(1)

	keyfrom, _, err := kmgr.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	keyf := base.PubKey(keyfrom.PubKey)

	keyto, _, err := kmgr.Create("keyto", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	keyt := base.PubKey(keyto.PubKey)

	state := testutil.NewState(t, &types.Genesis{
		Accounts: []types.Account{
			types.Account{Address: base.Bytes(keyfrom.Address), Balance: balance, Nonce: nonce},
		},
	})

	app, err := app_.Create(state, testutil.Logger())
	require.NoError(t, err)

	{
		nonce := uint64(0)
		tx, err := txutil.BuildTx(kmgr, keyfrom.Name, testutil.KeyPasswd, nonce, &types.TxSend{
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.CheckTx(tx)
		require.Equal(t, uint32(0x3), resp.Code)
		require.True(t, resp.IsErr())
		require.False(t, resp.IsOK())
	}

	{
		nonce := uint64(1)
		tx, err := txutil.BuildTx(kmgr, keyfrom.Name, testutil.KeyPasswd, nonce, &types.TxSend{
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.CheckTx(tx)
		require.Equal(t, uint32(0x3), resp.Code)
		require.True(t, resp.IsErr())
		require.False(t, resp.IsOK())
	}

	{
		nonce := uint64(2)
		tx, err := txutil.BuildTx(kmgr, keyfrom.Name, testutil.KeyPasswd, nonce, &types.TxSend{
			From:   base.Bytes(keyf.Address()),
			To:     base.Bytes(keyt.Address()),
			Amount: 0,
		})
		require.NoError(t, err)
		resp := app.CheckTx(tx)
		require.Equal(t, uint32(0x0), resp.Code)
		require.False(t, resp.IsErr())
		require.True(t, resp.IsOK())
	}

}

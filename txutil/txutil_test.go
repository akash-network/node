package txutil_test

import (
	"testing"

	"github.com/ovrclk/photon/testutil"
	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/require"
)

func TestTxBuilder(t *testing.T) {

	const nonce = 1

	manager := testutil.KeyManager(t)

	keyfrom, _, err := manager.Create("keyfrom", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	keyto, _, err := manager.Create("keyto", testutil.KeyPasswd, testutil.KeyAlgo)
	require.NoError(t, err)

	send := &types.TxSend{
		From:   base.Bytes(keyfrom.Address),
		To:     base.Bytes(keyto.Address),
		Amount: 100,
	}

	txbytes, err := txutil.BuildTx(manager, keyfrom.Name, testutil.KeyPasswd, nonce, send)

	txp, err := txutil.NewTxProcessor(txbytes)
	require.NoError(t, err)

	require.NoError(t, txp.Validate())

	tx := txp.GetTx()

	require.Equal(t, []byte(keyfrom.Address), tx.Key.Address())

	rsend := tx.Payload.GetTxSend()
	require.NotNil(t, rsend)

	require.Equal(t, rsend.From, send.From)
	require.Equal(t, rsend.To, send.To)
	require.Equal(t, rsend.Amount, send.Amount)
}

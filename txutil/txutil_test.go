package txutil_test

import (
	"testing"

	"github.com/ovrclk/akash/testutil"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
	"github.com/stretchr/testify/require"
)

func TestTxBuilder_KeyManager(t *testing.T) {

	const nonce = 1

	signer, keyfrom := testutil.PrivateKeySigner(t)
	keyto := testutil.PrivateKey(t)

	send := &types.TxSend{
		From:   keyfrom.PubKey().Address().Bytes(),
		To:     keyto.PubKey().Address().Bytes(),
		Amount: 100,
	}

	txbytes, err := txutil.BuildTx(signer, nonce, send)

	txp, err := txutil.NewTxProcessor(txbytes)
	require.NoError(t, err)

	require.NoError(t, txp.Validate())

	tx := txp.GetTx()

	require.Equal(t, keyfrom.PubKey().Bytes(), tx.Key)

	rsend := tx.Payload.GetTxSend()
	require.NotNil(t, rsend)

	require.Equal(t, rsend.From, send.From)
	require.Equal(t, rsend.To, send.To)
	require.Equal(t, rsend.Amount, send.Amount)
}

func TestTxBuilder_KeySigner(t *testing.T) {
	const nonce = 1

	keyfrom := testutil.PrivateKey(t)
	keyto := testutil.PrivateKey(t)

	send := &types.TxSend{
		From:   base.Bytes(keyfrom.PubKey().Address()),
		To:     base.Bytes(keyto.PubKey().Address()),
		Amount: 100,
	}

	txbytes, err := txutil.BuildTx(txutil.NewPrivateKeySigner(keyfrom), nonce, send)

	txp, err := txutil.NewTxProcessor(txbytes)
	require.NoError(t, err)

	require.NoError(t, txp.Validate())

	tx := txp.GetTx()

	require.Equal(t, keyfrom.PubKey().Bytes(), tx.Key)

	rsend := tx.Payload.GetTxSend()
	require.NotNil(t, rsend)

	require.Equal(t, rsend.From, send.From)
	require.Equal(t, rsend.To, send.To)
	require.Equal(t, rsend.Amount, send.Amount)

}

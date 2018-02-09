package txutil_test

import (
	"testing"

	"github.com/ovrclk/photon/txutil"
	"github.com/ovrclk/photon/types"
	"github.com/ovrclk/photon/types/base"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/go-crypto/keys"
	"github.com/tendermint/go-crypto/keys/cryptostore"
	"github.com/tendermint/go-crypto/keys/storage/memstorage"
)

func TestTxBuilder(t *testing.T) {

	addrTo_txt := "88B51008DE2C61E3AE94D608BAF6F7246C1D91F4"
	addrTo := new(base.Bytes)
	require.NoError(t, addrTo.DecodeString(addrTo_txt))

	codec, err := keys.LoadCodec("english")
	require.NoError(t, err)

	manager := cryptostore.New(
		cryptostore.SecretBox,
		memstorage.New(),
		codec,
	)

	passwd := "0123456789"

	key, _, err := manager.Create("key1", passwd, "ed25519")
	require.NoError(t, err)

	send := &types.TxSend{
		From:   base.Bytes(key.Address),
		To:     *addrTo,
		Amount: 100,
	}

	txbytes, err := txutil.BuildTx(manager, key.Name, passwd, send)

	txp, err := txutil.NewTxProcessor(txbytes)
	require.NoError(t, err)

	require.NoError(t, txp.Validate())

	tx := txp.GetTx()

	require.Equal(t, []byte(key.Address), tx.Key.Address())

	rsend := tx.Payload.GetTxSend()
	require.NotNil(t, rsend)

	require.Equal(t, rsend.From, send.From)
	require.Equal(t, rsend.To, send.To)
	require.Equal(t, rsend.Amount, send.Amount)
}
